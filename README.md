# SmolMusic
A Telegram bot for downloading and organizing music. Still work in progress (but still useful!)

# Features
- [x] Downloading music from YouTube
- [x] Creating, deleting playlists
- [x] Viewing lyrics

# Work in progress
- [ ] Sharing playlists
- [ ] Searching YouTube without leaving Telegram
- [ ] Buttons for controlling bot without using commands

# Requirements
- A Telegram account (bot sends all audio files to this account and then forwards them to users)
- Installed ffmpeg and yt-dlp
- You're running Linux (bot is not tested on Windows, still WIP)

# Installation
- Make sure that yt-dlp and ffmpeg are installed
- Grab the latest release binary 
- Setup your environmental values:
    - `SMOL_MUSIC_TOKEN="<YOUR_TELEGRAM_BOT_TOKEN>"` - you can get a bot token using [@BotFather](https://t.me/BotFather)
    - `SMOL_MUSIC_STORAGE="<STORAGE_CHAT_ID>"` - run the bot after setting token env value and text it from your storage account. Bot will text back the chat ID that you should set this value to. Example value - `SMOL_MUSIC_STORAGE="1112224342"`
- Run the bot

# Usage
For now bot is controlled with commands (going to change this in the future)
- `/download <youtube link>` - downloads the specified video from youtube as an audio file and sends it to the user
- `/addtoplaylist <youtube link> <playlist name>` - downloads the specified video from youtube and adds it to the playlist
- `/playlist <playlist name>` - sends all songs in the requested playlist to the user
- `/loadplaylist <youtube link> <playlist name>` - adds all songs from the YouTube playlist to the specified playlist
- `/remove` - removes the song user replied to from the playlist
- `/removeplaylist <playlist name>` - removes user's playlist
- `/lyrics` - shows lyrics of the song user replied to
