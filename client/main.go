package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"luxonis-task/transport"
	"net"
	"os"
	"strings"
)

type Client struct {
	ConnType         string
	Port             string
	TransportHandler transport.Transport
	Host             string
	UnixAddr         string
}

func newClient(connType, port, host, unixAddr string, transportHandler transport.Transport) *Client {
	return &Client{
		ConnType:         connType,
		Port:             port,
		TransportHandler: transportHandler,
		Host:             host,
		UnixAddr:         unixAddr,
	}
}

func (c *Client) parseClientRequest() (string, transport.MessageType, error) {
	reader := bufio.NewReader(os.Stdin)

	input, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading from standard input:", err)
		return "", transport.INVALID, err
	}

	input = strings.TrimSuffix(input, "\n")

	if input == "" {
		return "", transport.INVALID, nil
	}

	requestParts := strings.Fields(input)

	switch requestParts[0] {
	case transport.REQ_DISCONNECT:
		return "", transport.DISCONNECT, nil
	case transport.REQ_GET_PLAYERS:
		return "", transport.GET_PLAYERS, nil
	case transport.REQ_INIT_GAME:
		if c.validateCommand(requestParts, 3) {
			return fmt.Sprintf("%s %s", requestParts[1], requestParts[2]), transport.INIT_GAME, nil
		} else {
			return "", transport.INVALID, nil
		}
	case transport.REQ_GIVE_UP:
		return "", transport.GIVE_UP, nil
	default:
		return input, transport.MESSAGE, nil
	}
}

func (c *Client) validateCommand(parts []string, expectedParts int) bool {
	return len(parts) == expectedParts
}

func (c *Client) connect() {
	var address string

	if c.ConnType == "tcp" {
		address = fmt.Sprintf("%s:%s", c.Host, c.Port)
	} else {
		address = c.UnixAddr
	}

	conn, err := net.Dial(c.ConnType, address)

	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}

	defer conn.Close()

	go func() {
		for {
			input, reqType, err := c.parseClientRequest()

			if err != nil || reqType == transport.INVALID {
				fmt.Println("Invalid request")
				continue
			}

			c.TransportHandler.SendMessage(conn, input, reqType)
		}
	}()

	for {
		msg, err := c.TransportHandler.ReceiveMessage(conn)

		if err != nil {
			log.Fatal(err)
		}

		switch msg.Type {
		case transport.DISCONNECT:
			fmt.Println(msg.Data)
			fmt.Println("Server terminated the connection")
			os.Exit(0)
		case transport.MESSAGE, transport.INVALID:
			fmt.Println(msg.Data)
		case transport.GAME_STARTED:
			fmt.Println(msg.Data)
			c.TransportHandler.SendMessage(conn, "starting a new game", transport.GAME_STARTED)
		case transport.GAME_TERMINATED, transport.GIVE_UP:
			fmt.Println(msg.Data)
			c.TransportHandler.SendMessage(conn, "game over confirmed", transport.GAME_TERMINATED)
		default:
			fmt.Println("Receive invalid message type from server")
		}

	}

}

func main() {

	var (
		connType string
		port     string
		host     string
		unixAddr string
	)

	flag.StringVar(&connType, "type", "", "The type of connection (required | expect tcp or unix)")
	flag.StringVar(&host, "host", "localhost", "The host for TCP connection")
	flag.StringVar(&port, "port", "8080", "The port of TCP connection")
	flag.StringVar(&unixAddr, "socketAddr", "/tmp/luxonis-task", "The socket address")

	flag.Parse()

	if connType == "" {
		fmt.Fprintln(os.Stderr, "Error: Connection type is required for client.")
		os.Exit(2)
	}

	if connType != "tcp" && connType != "unix" {
		fmt.Fprintln(os.Stderr, "Error: Connection type must be either 'tcp' or 'unix'.")
		os.Exit(2)
	}

	transportHandler := transport.TransportHandler{}

	client := newClient(connType, port, host, unixAddr, transportHandler)

	client.connect()

}
