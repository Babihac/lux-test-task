package transport

import (
	"encoding/binary"
	"io"
	"net"
)

const (
	HEADER_LENGTH       = 6
	HEADER_LENGTH_BYTES = 4
	TYPE_LENGTH_BYTES   = 2
)

type MessageType uint16

type Message struct {
	Type MessageType
	Data string
}

const (
	DISCONNECT MessageType = iota
	GET_PLAYERS
	INIT_GAME
	INVALID
	MESSAGE
	GIVE_UP
	GAME_STARTED
	GAME_TERMINATED
)

const (
	REQ_DISCONNECT  = "exit"
	REQ_GET_PLAYERS = "list_players"
	REQ_INIT_GAME   = "start_game"
	REQ_GIVE_UP     = "give_up"
)

type Transport interface {
	SendMessage(conn net.Conn, message string, messageType MessageType) error
	ReceiveMessage(conn net.Conn) (*Message, error)
}

type TransportHandler struct{}

func (t TransportHandler) SendMessage(conn net.Conn, message string, messageType MessageType) error {
	data := []byte(message)

	header := make([]byte, HEADER_LENGTH)

	binary.BigEndian.PutUint32(header[0:HEADER_LENGTH_BYTES], uint32(len(data)))
	binary.BigEndian.PutUint16(header[HEADER_LENGTH_BYTES:HEADER_LENGTH], uint16(messageType))

	_, err := conn.Write(header)
	if err != nil {
		return err
	}

	_, err = conn.Write(data)
	if err != nil {
		return err
	}

	return nil

}

func (t TransportHandler) ReceiveMessage(conn net.Conn) (*Message, error) {
	header := make([]byte, HEADER_LENGTH)

	_, err := io.ReadFull(conn, header)

	if err != nil {
		return nil, err
	}

	dataLen := binary.BigEndian.Uint32(header[0:HEADER_LENGTH_BYTES])
	messageType := binary.BigEndian.Uint16(header[HEADER_LENGTH_BYTES:HEADER_LENGTH])

	data := make([]byte, dataLen)

	_, err = io.ReadFull(conn, data)
	if err != nil {
		return nil, err
	}

	message := &Message{
		Type: MessageType(messageType),
		Data: string(data),
	}

	return message, nil

}
