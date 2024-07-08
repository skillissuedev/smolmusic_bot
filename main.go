package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var storage_chat_id int64 = 0
var playlist_path = get_file_path("playlist.json")
var playlists = make(map[string][]Song)
var download_dir = get_file_path("")
var current_download_jobs = make([]string, 0)

func get_file_path(file string) string {
    home, err := os.UserHomeDir()
    if err != nil {
        log.Panic("Failed to get user home dir!")
    }

    smolmusic_path := home + "/smolmusic"
    os.MkdirAll(smolmusic_path, os.ModePerm)
    fmt.Println("smolmusic path:", home)

    file_path := smolmusic_path + "/" + file
    _, file_err := os.Stat(file_path)
    if os.IsNotExist(file_err) {
        os.Create(file_path)
        fmt.Println("Creating file " + file_path)
    } else {
        fmt.Println("File " + file_path + " exists")
    }

    return file_path
}

func main() {
    playlist_file, playlist_err := os.ReadFile(playlist_path)
    if playlist_err != nil {
        fmt.Println("main(): Failed to read the playlist file\nErr:", playlist_err.Error())
    }
    json_unmarshal_err := json.Unmarshal(playlist_file, &playlists)
    if json_unmarshal_err != nil {
        fmt.Println("main(): failed to unmarshal the json data!\nErr:", json_unmarshal_err.Error())
    }

    token := os.Getenv("SMOL_MUSIC_TOKEN")

    st_chat_id, st_chat_id_err := strconv.ParseInt(os.Getenv("SMOL_MUSIC_STORAGE"), 10, 64)
    echo_id_mode := false
    
    if st_chat_id_err != nil {
        fmt.Println("Failed to get/convert the SMOL_MUSIC_STORAGE env value to a integer")
        fmt.Println("Text the bot to get ChatID and set the SMOL_MUSIC_STORAGE value to it")
        fmt.Println("Make sure that your SMOL_MUSIC_TOKEN value is set to a valid Telegram bot token")
        echo_id_mode = true
    }
    storage_chat_id = st_chat_id

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
    if echo_id_mode {
        for update := range updates {
            if update.Message != nil { // If we got a message
                chat_id := update.Message.Chat.ID

                message := tgbotapi.NewMessage(chat_id, "Chat ID is " + fmt.Sprint(chat_id))
                bot.Send(message)
                return
            }
        }
    }

	for update := range updates {
		if update.Message != nil { // If we got a message
            args := update.Message.CommandArguments()
            msg_id := update.Message.MessageID
            chat_id := update.Message.Chat.ID
            fmt.Println("New message! Chat ID:", chat_id)

            if update.Message.Command() == "download" {
                go download(args, "", bot, chat_id, msg_id, true, 0, true)
            } else if update.Message.Command() == "addtoplaylist" {
                args_list := strings.Split(args, " ")
                if len(args_list) != 2 {
                    message_text := "Right way to use this command: /addtoplaylist <youtube link> <your playlist name>"

                    message := tgbotapi.NewMessage(chat_id, message_text)
                    message.ReplyToMessageID = msg_id
                    bot.Send(message)
                    continue
                }

                url := args_list[0]
                playlist := strings.ReplaceAll(args_list[1], "_", "")
                go add_to_playlist(url, playlist, bot, chat_id, msg_id, true)
            } else if update.Message.Command() == "loadplaylist" {
                args_list := strings.Split(args, " ")
                if len(args_list) != 2 {
                    message_text := "Right way to use this command: /loadplaylist <youtube playlist link> <your playlist name>"

                    message := tgbotapi.NewMessage(chat_id, message_text)
                    message.ReplyToMessageID = msg_id
                    bot.Send(message)
                    continue
                }

                url := args_list[0]
                playlist := strings.ReplaceAll(args_list[1], "_", "")
                go load_playlist(url, playlist, bot, chat_id, msg_id)
            } else if update.Message.Command() == "playlist" {
                args_list := strings.Split(args, " ")
                if len(args_list) != 1 {
                    message_text := "Right way to use this command: /playlist <your playlist name>"

                    message := tgbotapi.NewMessage(chat_id, message_text)
                    message.ReplyToMessageID = msg_id
                    bot.Send(message)
                    continue
                }

                playlist_name := args_list[0]
                go playlist(playlist_name, bot, chat_id, msg_id)
            } else if update.Message.Command() == "remove" {
                if update.Message.ReplyToMessage == nil {
                    message_text := "To remove a song from a playlist use this command while replying to it"

                    message := tgbotapi.NewMessage(chat_id, message_text)
                    message.ReplyToMessageID = msg_id
                    bot.Send(message)
                    continue
                }
                go remove(update.Message.ReplyToMessage, bot, chat_id, msg_id)
            } else if update.Message.Command() == "lyrics" {
                if update.Message.ReplyToMessage == nil {
                    message_text := "To get lyrics of the song, use this command while replying to it"

                    message := tgbotapi.NewMessage(chat_id, message_text)
                    message.ReplyToMessageID = msg_id
                    bot.Send(message)
                    continue
                }

                go lyrics(update.Message.ReplyToMessage, bot, chat_id, msg_id)
            } else if update.Message.Command() == "removeplaylist" {
                args_list := strings.Split(args, " ")
                if len(args_list) != 1 {
                    message_text := "Right way to use this command: /removeplaylist <your playlist name>"

                    message := tgbotapi.NewMessage(chat_id, message_text)
                    message.ReplyToMessageID = msg_id
                    bot.Send(message)
                    continue
                }

                go remove_playlist(args_list[0], bot, chat_id, msg_id)
            }
        }
    }
}

func download(url string, playlist string, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int, extract_id bool, attempt uint, should_print bool) (*int, *string) {
    var start_message_sended *tgbotapi.Message
    var start_message_error *error
    start_message_text := "Starting download of a video '" + url + "'"
    
    if should_print == true {
        start_message := tgbotapi.NewMessage(chat_id, start_message_text)
        start_message.ReplyToMessageID = reply_to_message_id
        start_message_sended_owned, start_message_err_owned := bot.Send(start_message)
        start_message_sended = &start_message_sended_owned
        start_message_error = &start_message_err_owned
    }


    var vid_id string
    if extract_id == true {
        vid_id_bytes, vid_id_err := exec.Command("yt-dlp", "--get-id", url, "--no-playlist", "-i").Output()
        if vid_id_err != nil {
            fmt.Println("download(): running yt-dlp to get-id failed!\nErr: ", vid_id_err.Error())
            if attempt > 3 {
                return nil, nil
            }
            fmt.Println("download(): retrying, attempt #" + fmt.Sprint(attempt + 1))
            download(url, playlist, bot, chat_id, reply_to_message_id, extract_id, attempt + 1, false)
            return nil, nil
        }
        vid_id = strings.ReplaceAll(string(vid_id_bytes), "\n", "")
    } else {
        vid_id = url
    }

    if vid_id == "" {
        return nil, nil
    }

    if should_print == true && start_message_error == nil {
        id := start_message_sended.MessageID
        edit_start_message_text := start_message_text + "\nExtracted video id - '" + vid_id + "'"
        progress_message_edit := tgbotapi.NewEditMessageText(chat_id, id, edit_start_message_text)
        bot.Send(progress_message_edit)
    }

    ytdlp_out_path := download_dir + vid_id + "_" + playlist + ".mp4"

    if slices.Contains(current_download_jobs, ytdlp_out_path) {
        abort_message_text := "This video is already downloading, aborting!"
        abort_message := tgbotapi.NewMessage(chat_id, abort_message_text)

        if start_message_error == nil {
            abort_message.ReplyToMessageID = start_message_sended.MessageID
        }

        bot.Send(abort_message)
        return nil, nil
    }
    current_download_jobs = append(current_download_jobs, ytdlp_out_path)

    fmt.Println("download's output is ", ytdlp_out_path)
    ytdlp_output, ytdlp_err := exec.Command("yt-dlp", "-S res:144", "--remux-video", "mp4", "--merge-output-format", "mp4", "--embed-metadata", "-o", ytdlp_out_path, "http://youtube.com/watch?v=" + vid_id).Output()

    if ytdlp_err != nil {
        fmt.Println(string(ytdlp_output))
        fmt.Println("download(): running yt-dlp failed!\nErr: ", ytdlp_err.Error())
        fmt.Println("yt-dlp", "-S res:144", "--embed-metadata", "-o", ytdlp_out_path, "http://youtube.com/watch?v=" + vid_id)
        remove_job(ytdlp_out_path)
        if attempt > 3 {
            return nil, nil
        }
        fmt.Println("download(): retrying, attempt #" + fmt.Sprint(attempt + 1))
        os.Remove(ytdlp_out_path)
        download(url, playlist, bot, chat_id, reply_to_message_id, extract_id, attempt + 1, false)
    }

    ffmpeg_out_path := download_dir + vid_id + "_" + playlist + ".mp3"
    _, ffmpeg_err := exec.Command("ffmpeg", "-i", ytdlp_out_path, "-vn", ffmpeg_out_path).Output()

    if ffmpeg_err != nil {
        fmt.Println("download(): running ffmpeg failed!\nErr: ", ffmpeg_err.Error())
        remove_job(ytdlp_out_path)
        if attempt > 3 {
            return nil, nil
        }
        fmt.Println("download(): retrying, attempt #" + fmt.Sprint(attempt + 1))
        os.Remove(ytdlp_out_path)
        os.Remove(ffmpeg_out_path)
        download(url, playlist, bot, chat_id, reply_to_message_id, extract_id, attempt + 1, false)
        return nil, nil
    }

    message := tgbotapi.NewAudio(storage_chat_id, tgbotapi.FilePath(ffmpeg_out_path))
    sent_message, sent_message_err := bot.Send(message)
    if sent_message_err != nil {
        fmt.Println("download(): sending message to a storage account failed!\nErr: ", sent_message_err.Error())
        fmt.Println("ffmpeg path is", ffmpeg_out_path)
        remove_job(ytdlp_out_path)
        return nil, nil
    }

    forward_message := tgbotapi.NewForward(chat_id, storage_chat_id, sent_message.MessageID)
    forward_message.ReplyToMessageID = reply_to_message_id
    bot.Send(forward_message)
    os.Remove(ytdlp_out_path)
    os.Remove(ffmpeg_out_path)

    file_name := sent_message.Audio.FileName

    remove_job(ytdlp_out_path)

    return &sent_message.MessageID, &file_name
}

func add_to_playlist(url string, playlist string, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int, extract_id bool) {
    start_message := tgbotapi.NewMessage(chat_id, "Adding a video with URL '" + url + "' in the playlist '" + playlist + "'")
    start_message.ReplyToMessageID = reply_to_message_id
    bot.Send(start_message)

    playlist_name := fmt.Sprint(chat_id) + playlist
    message_id, file_name := download(url, playlist_name, bot, chat_id, reply_to_message_id, extract_id, 0, false)

    if message_id == nil || file_name == nil {
        return
    }

    playlist_array := playlists[playlist_name]
    playlist_array = append(playlist_array, Song { *message_id, *file_name })
    playlists[playlist_name] = playlist_array

    json_playlists, json_playlists_error := json.Marshal(playlists)
    if json_playlists_error != nil {
        fmt.Println("add_to_playlist(): failed to marshal the json data!\nErr:", json_playlists_error.Error())
    }

    write_err := os.WriteFile(playlist_path, json_playlists, os.ModePerm)
    if write_err != nil {
        fmt.Println("add_to_playlist(): failed to write the file!\nErr:", write_err.Error())
    }
}

func playlist(playlist string, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int) {
    playlist_name := fmt.Sprint(chat_id) + playlist
    playlist_array := playlists[playlist_name]

    if len(playlist_array) == 0 {
        empty_playlist_message := tgbotapi.NewMessage(chat_id, "Playlist not found! Create one with /addtoplaylist command.")
        empty_playlist_message.ReplyToMessageID = reply_to_message_id
        bot.Send(empty_playlist_message)
        return
    }

    start_message := tgbotapi.NewMessage(chat_id, "Playlist '" + playlist + "':")
    start_message.ReplyToMessageID = reply_to_message_id
    bot.Send(start_message)
    for i := 0; i < len(playlist_array); i++ {
        message_id := playlist_array[i].MessageID
        forward := tgbotapi.NewForward(chat_id, storage_chat_id, message_id)
        bot.Send(forward)
    }
}

func remove(reply_message *tgbotapi.Message, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int) {
    if reply_message.Audio == nil {
        message_text := "To remove a song from a playlist use this command while replying to it"

        message := tgbotapi.NewMessage(chat_id, message_text)
        message.ReplyToMessageID = reply_to_message_id
        bot.Send(message)
        return
    }

    /*
    playlist_file, playlist_err := os.ReadFile(playlist_path)
    if playlist_err != nil {
        fmt.Println("remove(): Failed to read the playlist file\nErr:", playlist_err.Error())
    }
    playlist_data := make(map[string][]Song)
    json_unmarshal_err := json.Unmarshal(playlist_file, &playlist_data)
    if json_unmarshal_err != nil {
        fmt.Println("remove(): failed to unmarshal the json data!\nErr:", json_unmarshal_err.Error())
    }*/

    audio_file_name := reply_message.Audio.FileName
    split_file_name := strings.Split(audio_file_name, "_")
    playlist_name := strings.ReplaceAll(split_file_name[len(split_file_name) - 1], ".mp3", "")
    fmt.Println(playlist_name)
    playlist := playlists[playlist_name]
    ids_to_remove := make([]int, 0)
    for i := 0; i < len(playlist); i++ {
        if playlist[i].FileName == audio_file_name {
            ids_to_remove = append(ids_to_remove, i)
        }
    }
    fmt.Println(ids_to_remove)
    fmt.Println(playlist)
    for i := 0; i < len(ids_to_remove); i++ {
        if len(playlist) > 1 {
            id := ids_to_remove[i]
            playlist = slices.Delete(playlist, id, id + 1)
        } else {
            playlist = make([]Song, 0)
        }

        for j := 0; j < len(ids_to_remove); j++ {
            ids_to_remove[j] -= 1
        }
    }

    fmt.Println(playlist)
    playlists[playlist_name] = playlist


    json_playlists, json_playlists_error := json.Marshal(playlists)
    if json_playlists_error != nil {
        fmt.Println("remove(): failed to marshal the json data!\nErr:", json_playlists_error.Error())
    }
    write_err := os.WriteFile(playlist_path, json_playlists, os.ModePerm)
    if write_err != nil {
        fmt.Println("remove(): failed to write the file!\nErr:", write_err.Error())
    }

    message := tgbotapi.NewMessage(chat_id, "Successfully removed the song!")
    message.ReplyToMessageID = reply_to_message_id
    bot.Send(message)
}

type Song struct {
    MessageID int
    FileName string
}

func load_playlist(url string, playlist string, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int) {
    vid_ids_bytes, vid_ids_err := exec.Command("yt-dlp", "--flat-playlist", "--print", "id", url).Output()
    if vid_ids_err != nil {
        fmt.Println("load_playlis(): running yt-dlp to get-id failed!\nErr: ", vid_ids_err.Error())
    }
    vid_ids := strings.Split(string(vid_ids_bytes), "\n")

    message := tgbotapi.NewMessage(chat_id, "Getting IDs of all songs in your playlist, please wait!")
    message.ReplyToMessageID = reply_to_message_id
    bot.Send(message)

    for _,id := range vid_ids {
        if strings.Contains(id, " ") == false {
            if id != "" {
                fmt.Println("load_playlist(): adding video", id)
                go add_to_playlist(id, playlist, bot, chat_id, reply_to_message_id, false)
            }
        }
    }
}

func remove_job(req_job string) {
    for idx,job := range current_download_jobs {
        if job == req_job {
            current_download_jobs = slices.Delete(current_download_jobs, idx, idx + 1)
        }
    }
}

func remove_playlist(playlist string, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int) {
    start_message := tgbotapi.NewMessage(chat_id, "Removing playlist '" + playlist + "'!")
    start_message.ReplyToMessageID = reply_to_message_id
    bot.Send(start_message)

    playlist_name := fmt.Sprint(chat_id) + playlist
    delete(playlists, playlist_name)

    json_playlists, json_playlists_error := json.Marshal(playlists)
    if json_playlists_error != nil {
        fmt.Println("add_to_playlist(): failed to marshal the json data!\nErr:", json_playlists_error.Error())
    }

    write_err := os.WriteFile(playlist_path, json_playlists, os.ModePerm)
    if write_err != nil {
        fmt.Println("add_to_playlist(): failed to write the file!\nErr:", write_err.Error())
    }
}

func lyrics(reply_message *tgbotapi.Message, bot *tgbotapi.BotAPI, chat_id int64, reply_to_message_id int) {
    if reply_message.Audio == nil {
        message_text := "To get song's lyrics use this command while replying to it"

        message := tgbotapi.NewMessage(chat_id, message_text)
        message.ReplyToMessageID = reply_to_message_id
        bot.Send(message)
        return
    }
    artist := reply_message.Audio.Performer
    name := reply_message.Audio.Title
    req_response, req_err := http.Get("https://lyrix.vercel.app/getLyricsByName/" + artist + "/" + name)

    if req_err != nil {
        fmt.Println("lyrics(): Request error!", req_err.Error())
        failed_message := tgbotapi.NewMessage(chat_id, "Failed to get song's lyrics!")
        failed_message.ReplyToMessageID = reply_to_message_id
        bot.Send(failed_message)
        return
    }
    body, err := io.ReadAll(req_response.Body)

    if err != nil {
        fmt.Println("lyrics(): Failed to read body of the response! The song is ", name, "by", artist, "Err:", err.Error())
        failed_message := tgbotapi.NewMessage(chat_id, "Failed to get song's lyrics!")
        failed_message.ReplyToMessageID = reply_to_message_id
        bot.Send(failed_message)
        return
    }

    lyrics_response := ""
    for _,json_line := range strings.Split(string(body), "\n") {
        if strings.Contains(json_line, "\"words\"") {
            line_parts := strings.Split(json_line, "\"") // line_parts[3] is an actual line from the song
            line := line_parts[3]
            line = strings.ReplaceAll(line, "♪", "\n")
            lyrics_response = lyrics_response + "\n" + line
        }
    }

    var message tgbotapi.MessageConfig
    if lyrics_response == "" {
        message = tgbotapi.NewMessage(chat_id, "Unable to get lyrics of this song!")
    } else {
        message = tgbotapi.NewMessage(chat_id, lyrics_response)
    }
    message.ReplyToMessageID = reply_to_message_id
    bot.Send(message)
}

type Lyrics struct {
    lines []map[string]string
}
