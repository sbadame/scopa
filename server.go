package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sbadame/scopa/scopa"
	"golang.org/x/net/websocket"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var (
	port   = flag.Int("port", 8080, "The port to listen on for requests.")
	random = flag.Bool("random", false, "When set to true, actually uses a random seed.")
)

// To test with no random seed:
// $ curl -v localhost:8080/join
// $ curl -v localhost:8080/state
// $ curl -v localhost:8080/take -H "Content-Type: application/json" --data '{"PlayerId": 1, "Card": {"Suite":4, "Value":2}, "Table": [{"Suite": 1, "Value": 2}]}'
// $ curl -v localhost:8080/drop -H "Content-Type: application/json" --data '{"Card": {"Suite":4, "Value":2}}'

type drop struct {
	PlayerId int
	Card     scopa.Card
}

type take struct {
	PlayerId int
	Card     scopa.Card
	Table    []scopa.Card
}

func parseRequestJson(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	c, err := strconv.Atoi(r.Header["Content-Length"][0])
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, fmt.Sprintf("Couldn't get the content length: %v", err))
		return false
	}

	buf := make([]byte, c)
	if _, err := io.ReadFull(r.Body, buf); err != nil {
		w.WriteHeader(500)
		io.WriteString(w, fmt.Sprintf("Error reading data: %v", err))
		return false
	}

	if err := json.Unmarshal(buf, &v); err != nil {
		w.WriteHeader(400)
		io.WriteString(w, fmt.Sprintf("Error parsing json: %v", err))
		return false
	}
	return true
}

// Keep track of the number of players that have "joined".
// Give them a player id.
var playerCount int
var playerMux sync.Mutex

func allocatePlayerId() (int, error) {
	playerMux.Lock()
	defer playerMux.Unlock()

	if playerCount >= 2 {
		return -1, fmt.Errorf("{\"Message\": \"Game is full!\"}")
	}

	playerCount += 1
	return playerCount, nil
}

func main() {
	flag.Parse()
	if *random {
		rand.Seed(time.Now().Unix())
	}

	state := scopa.NewGame()
	gameId := time.Now().Unix()
	clients := make([]chan struct{}, 0)

	// Serve resources for testing.
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// Reset the game...
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		state = scopa.NewGame()
		playerCount = 0
		gameId = time.Now().Unix()
		clients = make([]chan struct{}, 0)
	})

	http.Handle("/join", websocket.Handler(func(ws *websocket.Conn) {
		if playerId, err := allocatePlayerId(); err != nil {
			ws.Close()
			return
		} else {
			io.WriteString(ws, fmt.Sprintf("{\"Type\": \"INIT\", \"PlayerId\": %d, \"GameId\": %d}", playerId, gameId))
		}

		// "Register" a channel to listen to changes to.
		update := make(chan struct{}, 1000)
		clients = append(clients, update)

		// Push the initial state, then keep pushing a state
		for {
			u := struct {
				Type  string
				State scopa.State
			}{
				Type:  "STATE",
				State: state,
			}
			if j, err := json.Marshal(u); err != nil {
				io.WriteString(ws, fmt.Sprintf("{\"Type\": \"ERROR\", \"Message\": \"%v\"}", err))
			} else {
				ws.Write(j)
			}
			<-update
		}
	}))

	http.HandleFunc("/drop", func(w http.ResponseWriter, r *http.Request) {
		var d drop
		if !parseRequestJson(w, r, &d) {
			return
		}

		if d.PlayerId != state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, "{\"Message\": \"Not your turn!\"}")
			return
		}

		if err := state.Drop(d.Card); err != nil {
			w.WriteHeader(500)
			io.WriteString(w, fmt.Sprintf("Couldn't drop: %v", err))
		}

		// Update all of the clients, that there is some new state.
		for _, u := range clients {
			var s struct{}
			u <- s
		}
	})

	http.HandleFunc("/take", func(w http.ResponseWriter, r *http.Request) {
		var t take
		if !parseRequestJson(w, r, &t) {
			return
		}

		if t.PlayerId != state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, "{\"Message\": \"Not your turn!\"}")
			return
		}

		if err := state.Take(t.Card, t.Table); err != nil {
			w.WriteHeader(500)
			io.WriteString(w, fmt.Sprintf("Couldn't take: %v", err))
		}

		// Update all of the clients, that there is some new state.
		for _, u := range clients {
			var s struct{}
			u <- s
		}
	})

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
