package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nhooyr.io/websocket"
)

var players = make(chan Player)

type Player interface {
	GetSocket() *websocket.Conn
	GetPlayerId() string
	Moves() *chan string
}

type Client struct {
	connection *websocket.Conn
	sessionid  string
	moves      *chan string
}

func (c Client) GetSocket() *websocket.Conn {
	return c.connection
}
func (c Client) GetPlayerId() string {
	return c.sessionid
}
func (c Client) Moves() *chan string {
	return c.moves
}

type Square struct {
	piece    string
	side     string
	hasMoved bool
}

type Board struct {
	pieces [8][8]Square
}

type State struct {
	sequenceNumber int64
	board          Board
	whiteTimer     <-chan int
	blackTimer     <-chan int
}

func initializeBoard() State {
	var s State
	s.sequenceNumber = 0
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			s.board.pieces[i][j].hasMoved = false
			s.board.pieces[i][j].side = "None"
			s.board.pieces[i][j].piece = "Empty"
		}
	}

	for i := 0; i < 8; i++ {
		// set color of powerful pieces
		s.board.pieces[0][i].side = "Black"
		s.board.pieces[7][i].side = "White"

		s.board.pieces[1][i].side = "Black"
		s.board.pieces[1][i].piece = "Pawn"

		s.board.pieces[6][i].side = "White"
		s.board.pieces[6][i].piece = "Pawn"
	}

	for _, i := range []int{0, 7} {
		s.board.pieces[i][0].piece = "Rook"
		s.board.pieces[i][1].piece = "Knight"
		s.board.pieces[i][2].piece = "Bishop"
		s.board.pieces[i][3].piece = "Queen"
		s.board.pieces[i][4].piece = "King"
		s.board.pieces[i][5].piece = "Bishop"
		s.board.pieces[i][6].piece = "Knight"
		s.board.pieces[i][7].piece = "Rook"
	}
	return s
}

type Move struct {
	source         [2]int
	dest           [2]int
	sequenceNumber int64
}

func applyMove(b *Board, m Move, whiteSide bool, stateSequenceNumber int64) (string, error) {
	if m.sequenceNumber != stateSequenceNumber+1 {
		fmt.Println(
			"My sequence number is : ",
			stateSequenceNumber,
			" sent sequence Number is : ",
			m.sequenceNumber,
		)
		return "", errors.New("Sequence Number is not in sequence :/")
	}

	src := b.pieces[m.source[0]][m.source[1]]
	fmt.Println(
		"apply move: m src ",
		m.sequenceNumber,
		m.source[0],
		m.source[1],
		m.dest[0],
		m.dest[1],
		whiteSide,
		src.side,
		src.piece,
	)
	if (src.side == "White" && !whiteSide) || (src.side == "Black" && whiteSide) {
		return "", errors.New("You cant move the other side's pieces!")
	}
	if src.piece == "Empty" {
		return "", errors.New("You cant move an empty piece?!")
	}

	// require move switch case here (just move to destination for now)
	b.pieces[m.dest[0]][m.dest[1]].side = src.side
	b.pieces[m.dest[0]][m.dest[1]].piece = src.piece
	b.pieces[m.dest[0]][m.dest[1]].hasMoved = true
	b.pieces[m.source[0]][m.source[1]].side = "None"
	b.pieces[m.source[0]][m.source[1]].piece = "Empty"

	return "", nil
}

func sendState(ctx context.Context, p Player, s State) {
	// TODO: send state serialized!
	stateString := ""
	stateString += fmt.Sprintf("state %d ", s.sequenceNumber)
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			a := s.board.pieces[i][j].hasMoved
			b := s.board.pieces[i][j].side
			c := s.board.pieces[i][j].piece
			stateString += fmt.Sprintf("(%t, %s, %s) ", a, b, c)
		}
	}

	p.GetSocket().Write(ctx, websocket.MessageText, []byte(stateString))
}

func sendMessage(ctx context.Context, p Player, message string) {
	p.GetSocket().Write(ctx, websocket.MessageText, []byte(message))
}

func updateTimer(
	ctx context.Context,
	toggle <-chan int,
	countdownFrom int,
) (<-chan int, <-chan int) {
	whiteTimer := make(chan int)
	blackTimer := make(chan int)

	go func() {
		defer close(whiteTimer)
		defer close(blackTimer)

		whiteTicking := true
		whiteTime := countdownFrom
		blackTime := countdownFrom

		for {
			select {
			case <-ctx.Done():
				return
			case <-toggle:
				whiteTicking = !whiteTicking
			default:
				if whiteTicking {
					if whiteTime == 0 {
						return
					}
					whiteTime -= 1
				} else {
					if blackTime == 0 {
						return
					}
					blackTime -= 1
				}
				whiteTimer <- whiteTime
				blackTimer <- blackTime
			}
			time.Sleep(time.Second)
			// fmt.Println("Timers:", whiteTime, blackTime, whiteTicking)
		}
	}()

	return whiteTimer, blackTimer
}

func gameLoop(ctx context.Context, white *Player, black *Player) error {
	fmt.Printf("[gameLoop] go: startGame player1 addr: %p player2: %p\n", white, black)
	fmt.Println("[gameloop] started: initializing board")
	s := initializeBoard()
	timerToggler := make(chan int)
	// 10 minutes to each player
	s.whiteTimer, s.blackTimer = updateTimer(ctx, timerToggler, 60*10)

	turn := "white"
	fmt.Println("[gameloop] infinite for starting now...")
	for {
		select {
		case time := <-s.whiteTimer:
			if time == 0 {
				fmt.Println("White is out of time! Black wins!")
				return nil
			}
		case time := <-s.blackTimer:
			if time == 0 {
				fmt.Println("Black is out of time! White wins!")
				return nil
			}
		case move := <-*(*white).Moves():
			fmt.Println("white message received", move)

			if turn == "white" {
				fmt.Println("White move received!", move)

				if move == "needState" {
					sendState(ctx, *white, s)
					continue
				}
				m, err := parseMove(move)
				if err != nil {
					fmt.Println("white sent invalid move! err: ", err)
					sendMessage(ctx, (*white), "invalid move fmt")
					continue
				}
				gameWinner, err := applyMove(&(s.board), m, true, s.sequenceNumber)
				if gameWinner != "" {
					fmt.Println("White wins")
					break
				}
				if err == nil {
					s.sequenceNumber += 1
					turn = "black"
					timerToggler <- 1
					sendMessage(
						ctx,
						(*black),
						fmt.Sprintf(
							"othermove %d %d %d %d",
							m.source[0],
							m.source[1],
							m.dest[0],
							m.dest[1],
						),
					)
					sendMessage(
						ctx,
						(*white),
						fmt.Sprintf(
							"moveack %d %d %d %d",
							m.source[0],
							m.source[1],
							m.dest[0],
							m.dest[1],
						),
					)
				} else {
					fmt.Println(err)
					sendState(ctx, *white, s)
				}

			} else {
				fmt.Println("White sent move when it was not their turn!", move)
				sendState(ctx, *white, s)
				sendMessage(ctx, *white, "notYourTurn")
			}
		case move := <-*(*black).Moves():
			fmt.Println("black message received", move)
			if turn == "black" {
				fmt.Println("Black move received!", move)

				if move == "needState" {
					sendState(ctx, *black, s)
					continue
				}
				m, err := parseMove(move)
				if err != nil {
					fmt.Println("black sent invalid move! err: ", err)
					sendMessage(ctx, (*black), "invalid move fmt")
					continue
				}
				gameWinner, err := applyMove(&(s.board), m, false, s.sequenceNumber)
				if gameWinner != "" {
					fmt.Println("black wins")
					break
				}
				if err == nil {
					s.sequenceNumber += 1
					turn = "white"
					timerToggler <- 1
					sendMessage(
						ctx,
						(*white),
						fmt.Sprintf(
							"othermove %d %d %d %d",
							m.source[0],
							m.source[1],
							m.dest[0],
							m.dest[1],
						),
					)
					sendMessage(
						ctx,
						(*black),
						fmt.Sprintf(
							"moveack %d %d %d %d",
							m.source[0],
							m.source[1],
							m.dest[0],
							m.dest[1],
						),
					)
				} else {
					fmt.Println(err)
					sendState(ctx, *black, s)
				}

			} else {
				fmt.Println("Black sent move when it was not their turn!", move)
				sendMessage(ctx, *black, "notYourTurn")
				sendState(ctx, *black, s)
			}

		}

	}
}

func parseMove(move string) (Move, error) {
	splitted := strings.Split(move, " ")
	if len(splitted) != 5 {
		return Move{}, errors.New("Move format is not seq src_x src_y dest_x dest_y (len mismatch)")
	}
	seqNum, err := strconv.Atoi(splitted[0])
	if err != nil {
		return Move{}, errors.New("Move format is not seq")
	}
	src_x, err := strconv.Atoi(splitted[1])
	if err != nil {
		return Move{}, errors.New("Move format is not seq src_x")
	}
	src_y, err := strconv.Atoi(splitted[2])
	if err != nil {
		return Move{}, errors.New("Move format is not seq src_x src_y")
	}
	dest_x, err := strconv.Atoi(splitted[3])
	if err != nil {
		return Move{}, errors.New("Move format is not seq src_x src_y dest_x")
	}
	dest_y, err := strconv.Atoi(splitted[4])
	if err != nil {
		return Move{}, errors.New("Move format is not seq src_x src_y dest_x dest_y")
	}
	var m Move
	m.dest = [2]int{dest_x, dest_y}
	m.source = [2]int{src_x, src_y}
	m.sequenceNumber = int64(seqNum)

	return m, nil

}

func startGame(ctx context.Context, white *Player, black *Player) error {

	fmt.Printf("[autoStarter] go: startGame player1 addr: %p player2: %p\n", white, black)
	err := (*white).GetSocket().Write(ctx, websocket.MessageText, []byte("StartWhite"))
	if err != nil {
		fmt.Println("Player 1 start game send failed. ", err)
		return err
	}
	err = (*black).GetSocket().Write(ctx, websocket.MessageText, []byte("StartBlack"))
	if err != nil {
		fmt.Println("Player 2 start game send failed. ", err)
		return err
	}

	fmt.Println("[startGame] go: gameLoop")
	go gameLoop(ctx, white, black)

	return nil
}
