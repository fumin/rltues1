package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.HandleFunc("/TTT", TTT)
	http.HandleFunc("/TTTMove", TTTMove)
	http.HandleFunc("/Status", Status)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("%v", err)
	}
}

type Player int

const (
	none Player = iota
	circle
	cross
)

var playerName = []string{
	none:   "NONE",
	circle: "CIRCLE",
	cross:  "CROSS",
}

type Msg struct {
	Type string
	Body []byte
}

const (
	MsgTypeBoard  = "b"
	MsgTypeWinner = "w"
)

type TokenChan struct {
	Token string
	C     chan Msg
}

type Room struct {
	sync.RWMutex
	Board         []Player
	CurrentPlayer Player
	Circle        *TokenChan
	Cross         *TokenChan
}

func setChan(room *Room, player Player, tc *TokenChan) error {
	room.Lock()
	defer room.Unlock()

	if player == circle {
		if room.Circle != nil {
			return fmt.Errorf("circle is occupied")
		}
		room.Circle = tc
	} else {
		if room.Cross != nil {
			return fmt.Errorf("cross is occupied")
		}
		room.Cross = tc
	}

	return nil
}

func jsonBoard(board []Player) []byte {
	b, err := json.Marshal(board)
	if err != nil {
		log.Fatalf("%v", err)
	}
	return b
}

func move(room *Room, player Player, position int) error {
	room.Lock()
	defer room.Unlock()

	if room.CurrentPlayer != none && room.CurrentPlayer != player {
		return fmt.Errorf("not your turn")
	}
	if room.Board[position] != none {
		return fmt.Errorf("position occupied")
	}

	room.Board[position] = player
	if player == circle {
		room.CurrentPlayer = cross
	} else {
		room.CurrentPlayer = circle
	}

	win := winner(room.Board)
	if win != none {
		select {
		case room.Cross.C <- Msg{Type: MsgTypeWinner, Body: []byte(playerName[win])}:
		default:
		}
		select {
		case room.Circle.C <- Msg{Type: MsgTypeWinner, Body: []byte(playerName[win])}:
		default:
		}
	} else {
		if player == circle {
			if room.Cross != nil {
				select {
				case room.Cross.C <- Msg{Type: MsgTypeBoard, Body: jsonBoard(room.Board)}:
				default:
				}
			}
		} else {
			if room.Circle != nil {
				select {
				case room.Circle.C <- Msg{Type: MsgTypeBoard, Body: jsonBoard(room.Board)}:
				default:
				}
			}
		}
	}

	return nil
}

func winner(b []Player) Player {
	// Horizontal
	if b[0] != none && b[0] == b[1] && b[0] == b[2] {
		return b[0]
	}
	if b[3] != none && b[3] == b[4] && b[3] == b[5] {
		return b[3]
	}
	if b[6] != none && b[6] == b[7] && b[6] == b[8] {
		return b[6]
	}

	// Vertical
	if b[0] != none && b[0] == b[3] && b[0] == b[6] {
		return b[0]
	}
	if b[1] != none && b[1] == b[4] && b[1] == b[7] {
		return b[1]
	}
	if b[2] != none && b[2] == b[5] && b[2] == b[8] {
		return b[2]
	}

	// Diagonal
	if b[0] != none && b[0] == b[4] && b[0] == b[8] {
		return b[0]
	}
	if b[6] != none && b[6] == b[4] && b[6] == b[2] {
		return b[6]
	}

	return none
}

var rooms = struct {
	sync.RWMutex
	m map[string]*Room
}{m: make(map[string]*Room)}

func TTT(w http.ResponseWriter, r *http.Request) {
	roomName := r.FormValue("room")
	player := circle
	if r.FormValue("player") == "cross" {
		player = cross
	}

	rooms.Lock()
	room, ok := rooms.m[roomName]
	if !ok {
		room = &Room{
			Board:         make([]Player, 9),
			CurrentPlayer: none,
		}
		rooms.m[roomName] = room

		defer func() {
			rooms.Lock()
			delete(rooms.m, roomName)
			rooms.Unlock()
		}()
	}
	rooms.Unlock()

	tc := &TokenChan{
		Token: randToken(),
		C:     make(chan Msg),
	}
	if err := setChan(room, player, tc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sse := NewServerSideEventsWriter(w)
	if err := sse.EventWrite("token", []byte(tc.Token)); err != nil {
		return
	}
	if err := sse.EventWrite(MsgTypeBoard, jsonBoard(room.Board)); err != nil {
		return
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := sse.EventWrite("h", nil); err != nil {
				return
			}
		case msg := <-tc.C:
			if msg.Type == MsgTypeBoard {
				if err := sse.EventWrite(msg.Type, msg.Body); err != nil {
					return
				}
			} else if msg.Type == MsgTypeWinner {
				sse.EventWrite(msg.Type, msg.Body)
				return
			}
		}
	}
}

func TTTMove(w http.ResponseWriter, r *http.Request) {
	roomName := r.FormValue("room")
	rooms.RLock()
	room, ok := rooms.m[roomName]
	rooms.RUnlock()
	if !ok {
		http.Error(w, "no such room", http.StatusBadRequest)
		return
	}

	player := circle
	if r.FormValue("player") == "cross" {
		player = cross
	}
	token := r.FormValue("token")
	if player == circle && room.Circle.Token != token {
		http.Error(w, "wrong token", http.StatusBadRequest)
		return
	}
	if player == cross && room.Cross.Token != token {
		http.Error(w, "wrong token", http.StatusBadRequest)
		return
	}

	position, err := strconv.Atoi(r.FormValue("position"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := move(room, player, position); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func Status(w http.ResponseWriter, r *http.Request) {
	page := struct {
		Rooms int
	}{
		Rooms: len(rooms.m),
	}
	json.NewEncoder(w).Encode(page)
}

func randToken() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		panic(err.Error())
	}
	s := base64.URLEncoding.EncodeToString(b)
	return strings.Replace(s, "=", "", -1)
}

// ByteWriter is a one-off utility type for writing to http.ResponseWriter
type ByteWriter struct {
	RespWriter http.ResponseWriter
	Err        error
}

func (w *ByteWriter) Write(b []byte) {
	if w.Err != nil {
		return
	}
	_, w.Err = w.RespWriter.Write(b)
}

// A Sse is a wrapper over a Server-Sent Events response.
type Sse struct {
	w http.ResponseWriter
}

func NewServerSideEventsWriter(w http.ResponseWriter) Sse {
	headers := w.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	return Sse{w: w}
}

func (sse Sse) Write(b []byte) error {
	bw := &ByteWriter{RespWriter: sse.w}
	bw.Write([]byte("data: "))
	bw.Write(b)
	bw.Write([]byte("\n\n"))
	if bw.Err != nil {
		return bw.Err
	}
	if f, ok := sse.w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func (sse Sse) EventWrite(event string, b []byte) error {
	bw := &ByteWriter{RespWriter: sse.w}
	bw.Write([]byte("event: "))
	bw.Write([]byte(event))
	bw.Write([]byte("\n"))
	if bw.Err != nil {
		return bw.Err
	}
	return sse.Write(b)
}
