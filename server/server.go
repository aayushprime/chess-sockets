package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

func autoStarter() {
	for {
		gameCtx := context.Background()
		player1 := <-players
		player2 := <-players

		fmt.Printf("[autoStarter] go: startGame player1 addr: %p player2: %p\n", &player1, &player2)
		go startGame(gameCtx, &player1, &player2)
	}
}

type chessServer struct {
	logf func(f string, v ...interface{})
}

func (s *chessServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:   []string{"echo"},
		OriginPatterns: []string{"localhost:5173"},
	})
	if err != nil {
		s.logf("%v", err)
		return
	}
	// defer c.CloseNow()

	// if c.Subprotocol() != "echo" {
	// 	fmt.Println("Subprotocol is not echo!", c.Subprotocol())
	// 	c.Close(websocket.StatusPolicyViolation, "client must speak the echo subprotocol")
	// 	return
	// }

	l := rate.NewLimiter(rate.Every(time.Millisecond*100), 10)
	// client, err := authSocket(r.Context(), c, l)
	client, err := authSocket(context.Background(), c, l)
	if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
		return
	}
	if err != nil {
		s.logf("failed to echo with %v: %v", r.RemoteAddr, err)
		return
	}
	players <- client
	return
}

// try to read the first message from the socket and authorize it
func authSocket(
	ctx context.Context,
	c *websocket.Conn,
	l *rate.Limiter,
	// sessions map[string]Player,
) (Client, error) {
	err := l.Wait(ctx)
	if err != nil {
		return Client{}, err
	}
	typ, r, err := c.Reader(ctx)
	if err != nil {
		return Client{}, err
	}
	if typ != websocket.MessageText {
		return Client{}, errors.New("Send first message as <Token> <Session ID>")
	}
	buf, err := io.ReadAll(r)
	if err != nil {
		fmt.Println("[authSocket] Cannot read from socket.", err)
	}
	splitted := strings.Split(string(buf), " ")
	if splitted[0] != "TOKEN" {
		c.Close(websocket.StatusAbnormalClosure, "TOKEN KHOI")
		return Client{}, errors.New("Token mismatch")
	}
	var sessionId string
	// if len(splitted) >= 2 {
	// everything is new connection for now
	if false {
		// reconnection is not implemented
		// session, ok := sessions[splitted[1]]
		// if !ok {
		// 	sessionId = generateRandomString(8)
		// }
		//
		// var client Client
		// client.connection = c
		// client.sessionid = sessionId
		// go pumpMoves(client)
		return Client{}, nil

	} else {
		sessionId = generateRandomString(8)
		var client Client
		client.connection = c
		client.sessionid = sessionId
		client_moves_channel := make(chan string, 2)
		client.moves = &client_moves_channel

		go pushMoves(ctx, l, &client)
		return client, nil
	}
}

// loop (push moves to channel!)
func pushMoves(ctx context.Context, l *rate.Limiter, client *Client) {
	defer client.connection.Close(websocket.StatusTryAgainLater, "I cannot handle you right now.")
	for {
		err := l.Wait(ctx)
		if err != nil {
			fmt.Println("Rate limited socket: ", err)
			continue
		}
		typ, r, err := client.connection.Reader(ctx)
		if err != nil {
			fmt.Println("[pushMoves.Reader] Cannot read from socket:", err)
			return
		}
		if typ != websocket.MessageText {
			fmt.Println("Non-text message")
		}
		buf, err := io.ReadAll(r)
		if err != nil {
			fmt.Println("[pushMoves.ReadAll] Cannot read from socket:", err)
			return
		}
		fmt.Println("[pushMoves] Pushing moves.... ", string(buf))
		fmt.Printf(
			"[pushMoves] client pointer: %p, client moves pointer: %p\n",
			client,
			(*client).moves,
		)
		*((*client).moves) <- string(buf)
	}
}
