# Description

Test task containing server and client for guess a word game
I could only tested it on darwin/arm64, but I created build with this configuration:

```bash
 GOOS=linux GOARCH=amd64  go build -o
```

which should work on requested system. I also set Go version to 1.18, which was the default version after running apt install golang in Ubuntu container

## Installation

There are no external libraries. Only requirement is Golang

```bash
sudo apt install golang
```

## Usage

Start the server_app.

```bash
#check flags ./server_app -help
./server_app_x86 -pswd=YOUR_PASSWORD
```

start some clients

```bash
#check flags ./client_app -help
./client_app_x86 -type=tcp
./client_app_x86 -type=unix
```

You must enter the server's password and after that, server sends you your ID. Now you can start a game

```bash
list_players #this will return IDs of users who are waiting for game
start_game USER_ID YOUR_SECRET_WORD #this will start new game and send info to other user
```

You will be informed about other user progress and you can send him some hints.

If the other user wants to end current game he can just type command for it

```bash
give_up
```

When the game is over, Both players are free to start a new game again.
