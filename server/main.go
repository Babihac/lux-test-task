package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"luxonis-task/transport"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type PlayerState int

const (
	Connected PlayerState = iota
	Authenticated
	Playing
	Guessing
	Error
)

type Player struct {
	ID          string
	conn        net.Conn
	state       PlayerState
	currentGame *Game
}

type Game struct {
	ID         string
	Player     *Player
	Opponent   *Player
	SecretWord string
}

type Server struct {
	Players          map[string]*Player
	Games            map[string]*Game
	TransportHandler transport.Transport
	Password         string
	UnixAddr         string
	Host             string
	Port             string
	PlayersLock      sync.RWMutex
	GamesLock        sync.RWMutex
}

func (s *Server) handlePlayer(player Player) {

	for {
		switch player.state {
		case Connected:
			s.authenticatePlayer(&player)
		case Authenticated:
			s.pickOponentForPlayer(&player)
		case Playing:
			s.play(&player)
		case Guessing:
			s.guess(&player)
		case Error:
			player.conn.Close()
			return
		}

	}
}

func newServer(password string, unixAddr string, host string, port string, trasportHandler transport.Transport) *Server {
	return &Server{
		Password:         password,
		UnixAddr:         unixAddr,
		Host:             host,
		Port:             port,
		Players:          make(map[string]*Player),
		Games:            make(map[string]*Game),
		TransportHandler: trasportHandler,
	}
}

func newGame(player, opponent *Player, secretWord string) *Game {
	id := generateId()

	return &Game{
		ID:         id,
		Player:     player,
		Opponent:   opponent,
		SecretWord: secretWord,
	}
}

func (s *Server) disconnectPlayer(player *Player) {
	s.TransportHandler.SendMessage(player.conn, "", transport.DISCONNECT)
	s.PlayersLock.Lock()
	delete(s.Players, player.ID)
	s.PlayersLock.Unlock()
}

func (s *Server) initializeGame(player *Player, opponent *Player, secretWord string) {

	game := newGame(player, opponent, secretWord)

	s.GamesLock.Lock()
	s.Games[game.ID] = game
	s.GamesLock.Unlock()

	player.state = Playing
	opponent.state = Guessing

	player.currentGame = game
	opponent.currentGame = game

	s.TransportHandler.SendMessage(opponent.conn, fmt.Sprintf("Player %s has started game against you. Now guess his word...", player.ID), transport.GAME_STARTED)
	s.TransportHandler.SendMessage(player.conn, fmt.Sprintf("You started game against player %s You can send him some hints if he struggles with your word", opponent.ID), transport.GAME_STARTED)
}

func (s *Server) getFreePlayers(currentPlayer *Player) map[string]*Player {
	freePlayers := make(map[string]*Player)
	s.PlayersLock.Lock()
	for id, player := range s.Players {
		if player.currentGame == nil && currentPlayer.ID != player.ID {
			freePlayers[id] = player
		}
	}
	s.PlayersLock.Unlock()
	return freePlayers
}

func (s *Server) authenticatePlayer(player *Player) {
	s.TransportHandler.SendMessage(player.conn, "Enter secret Password!", transport.MESSAGE)

	msg, err := s.TransportHandler.ReceiveMessage(player.conn)

	if err != nil {
		s.disconnectPlayer(player)
		return
	}

	password := msg.Data

	if password == s.Password {
		var builder strings.Builder
		id := generateId()
		msg := fmt.Sprintf("Correct password! Your ID is: %s\n", id)
		builder.WriteString(msg)
		builder.WriteString("Now, you can pick opponent or just wait until someone picks you...\n")

		s.TransportHandler.SendMessage(player.conn, msg, transport.MESSAGE)

		player.state = Authenticated
		player.ID = id
		s.PlayersLock.Lock()
		s.Players[id] = player
		s.PlayersLock.Unlock()

	} else {
		s.TransportHandler.SendMessage(player.conn, "Password is incorrect... you will be disconnected.", transport.DISCONNECT)
		player.conn.Close()
	}

}

func (s *Server) pickOponentForPlayer(player *Player) {

	msg, err := s.TransportHandler.ReceiveMessage(player.conn)

	if err != nil {
		s.disconnectPlayer(player)
		return
	}

	fmt.Printf("this is message %s\n", msg.Data)

	switch msg.Type {
	case transport.DISCONNECT:
		s.disconnectPlayer(player)
	case transport.GET_PLAYERS:
		s.handleGetPlayersRequest(player)
	case transport.INIT_GAME:
		fields := strings.Fields(msg.Data)
		if len(fields) != 2 {
			s.TransportHandler.SendMessage(player.conn, "Invalid request", transport.INVALID)
			return
		}
		s.handleInitGame(player, fields[0], fields[1])
	case transport.GAME_STARTED:
		return
	default:
		s.TransportHandler.SendMessage(player.conn, "Invalid request", transport.INVALID)
	}

}

func (s *Server) handleGetPlayersRequest(player *Player) {
	var builder strings.Builder

	builder.WriteString("You can play against these users:\n")

	for _, player := range s.getFreePlayers(player) {
		builder.WriteString(fmt.Sprintf("%s\n", player.ID))
	}

	s.TransportHandler.SendMessage(player.conn, builder.String(), transport.MESSAGE)
}

func (s *Server) handleInitGame(player *Player, opponentId string, secretWord string) {
	freePlayers := s.getFreePlayers(player)

	if opponentId == player.ID {
		s.TransportHandler.SendMessage(player.conn, "You cannot play against yourself", transport.INVALID)
		return
	}

	s.PlayersLock.Lock()
	opponent, ok := freePlayers[opponentId]
	s.PlayersLock.Unlock()

	if !ok {
		s.TransportHandler.SendMessage(player.conn, fmt.Sprintf("Opponent with Id %s is is not free or does not exist\n", opponentId), transport.INVALID)
		return
	}

	s.initializeGame(player, opponent, secretWord)

}

func (s *Server) play(player *Player) {
	msg, err := s.TransportHandler.ReceiveMessage(player.conn)
	currentGamge := player.currentGame

	if err != nil {
		s.disconnectPlayer(player)
		s.terminateGame(player.currentGame)
		infoMsg := fmt.Sprintf("Game was terminated. Player probably disconnected from the server. Secret word was '%s' Pick another game\n", currentGamge.SecretWord)
		s.TransportHandler.SendMessage(currentGamge.Opponent.conn, infoMsg, transport.GAME_TERMINATED)
		return
	}

	if msg.Type == transport.GAME_TERMINATED || msg.Type == transport.GIVE_UP {
		return
	}

	s.TransportHandler.SendMessage(currentGamge.Opponent.conn, msg.Data, transport.MESSAGE)
}

func (s *Server) guess(player *Player) {
	msg, err := s.TransportHandler.ReceiveMessage(player.conn)
	currentGamge := player.currentGame

	if err != nil {
		s.disconnectPlayer(player)
		s.terminateGame(player.currentGame)
		s.TransportHandler.SendMessage(currentGamge.Player.conn, "Game was terminated. Opponent probably disconnected from the server. Pick another game\n", transport.GAME_TERMINATED)
		return
	}

	switch msg.Type {
	case transport.GAME_TERMINATED:
		return
	case transport.GIVE_UP:
		s.TransportHandler.SendMessage(currentGamge.Player.conn, "Oponent gave up. You can pick another game now", transport.GIVE_UP)
		s.TransportHandler.SendMessage(currentGamge.Opponent.conn, fmt.Sprintf("You gave up. The secret word was: %s. You can start another game now", currentGamge.SecretWord), transport.MESSAGE)
		s.terminateGame(currentGamge)
		return

	}

	currentGuess := msg.Data

	if strings.EqualFold(currentGamge.SecretWord, currentGuess) {
		s.TransportHandler.SendMessage(currentGamge.Opponent.conn, "This is correct! You have won! Pick another game if you wish\n", transport.MESSAGE)
		s.TransportHandler.SendMessage(currentGamge.Player.conn, "Opponent guessed your word correctly! Pick another game if you wish\n", transport.GAME_TERMINATED)
		s.terminateGame(currentGamge)

		return
	}

	s.TransportHandler.SendMessage(currentGamge.Opponent.conn, "This is not the secret word. Take another guess!\n", transport.MESSAGE)
	s.TransportHandler.SendMessage(currentGamge.Player.conn, fmt.Sprintf("Opponent guessed incorect word: %s\n", currentGuess), transport.MESSAGE)

}

func (s *Server) terminateGame(game *Game) {
	if game == nil {
		return
	}

	game.Player.restoreInitState()
	game.Opponent.restoreInitState()

	s.GamesLock.Lock()
	delete(s.Games, game.ID)
	s.GamesLock.Unlock()
}

func (p *Player) restoreInitState() {
	p.state = Authenticated
	p.currentGame = nil
}

func (s *Server) StartListen() {
	tcpAddr := fmt.Sprintf("%s:%s", s.Host, s.Port)
	tcpListener, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		fmt.Println("Error creating TCP listener:", err)
		os.Exit(2)
	}

	fmt.Println("TCP server listening on:", tcpAddr)

	go func() {
		defer tcpListener.Close()
		for {
			tcpConn, err := tcpListener.Accept()
			if err != nil {
				fmt.Println("Error accepting TCP connection:", err)
				continue
			}

			player := Player{
				conn:        tcpConn,
				ID:          "",
				state:       Connected,
				currentGame: nil,
			}

			go s.handlePlayer(player)
		}

	}()

	socketListener, err := net.Listen("unix", s.UnixAddr)

	if err != nil {
		fmt.Println("Error creating Unix listener:", err)
		os.Exit(2)
	}

	fmt.Println("Unix socket server listening on:", s.UnixAddr)

	go func() {
		defer os.Remove(s.UnixAddr)
		defer socketListener.Close()

		for {
			socketConn, err := socketListener.Accept()
			if err != nil {
				fmt.Println("Error accepting Socket connection:", err)
				return
			}

			player := Player{
				conn:  socketConn,
				ID:    "",
				state: Connected,
			}

			go s.handlePlayer(player)
		}

	}()

}

func (s *Server) handleInterupt() {
	os.Remove(s.UnixAddr)
}

func generateId() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	id := hex.EncodeToString(bytes)
	return string(id)
}

func main() {
	closeChan := make(chan os.Signal, 1)
	signal.Notify(closeChan, os.Interrupt, syscall.SIGTERM)
	var (
		password string
		host     string
		port     string
		unixAddr string
	)
	flag.StringVar(&password, "pswd", "", "The password for the server")
	flag.StringVar(&host, "host", "localhost", "The host for TCP connection")
	flag.StringVar(&port, "port", "8080", "The port of TCP connection")
	flag.StringVar(&unixAddr, "socketAddr", "/tmp/luxonis-task", "The socket address")

	flag.Parse()

	if password == "" {
		fmt.Fprintln(os.Stderr, "Error: The password for the server is required.")
		os.Exit(2)
	}

	transportHandler := transport.TransportHandler{}
	server := newServer(password, unixAddr, host, port, transportHandler)

	server.StartListen()

	<-closeChan
	fmt.Println("the server is shutting down")
	server.handleInterupt()
}
