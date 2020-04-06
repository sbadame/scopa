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

// /drop request content body json is marsheled into this struct.
type drop struct {
	PlayerId int
	Card     scopa.Card
}

// /take request content body json is marsheled into this struct.
type take struct {
	PlayerId int
	Card     scopa.Card
	Table    []scopa.Card
}

// Represents the move (drop or take) that was mde
type move struct {
	Type string
	Drop *drop
	Take *take
}

// /join streams updates with JSON marshalled from this type to clients.
type update struct {
	Type  string
	State scopa.State
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
	fmt.Printf("Player %d\n", playerCount)
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
	var stateMux sync.Mutex
	gameId := time.Now().Unix()
	clients := make([]chan move, 0)
	logs := make([]string, 0)

	// Serve resources for testing.
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// Reset the game...
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		state = scopa.NewGame()
		playerCount = 0
		gameId = time.Now().Unix()
		clients = make([]chan move, 0)
		logs = make([]string, 0)
	})

	// Get Debug logs to repro
	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		stateMux.Lock()
		defer stateMux.Unlock()
		for _, s := range logs {
			io.WriteString(w, s)
			io.WriteString(w, "\n")
		}
	})

	http.Handle("/join", websocket.Handler(func(ws *websocket.Conn) {
		fmt.Println("/join")
		if playerId, err := allocatePlayerId(); err != nil {
			ws.Close()
			return
		} else {
			io.WriteString(ws, fmt.Sprintf("{\"Type\": \"INIT\", \"PlayerId\": %d, \"GameId\": %d}", playerId, gameId))
		}

		// "Register" a channel to listen to changes to.
		updateChan := make(chan move, 1000)
		clients = append(clients, updateChan)

		// Push the initial state, then keep pushing a state + moves
		for {
			// Push the game state
			s := update{
				Type:  "STATE",
				State: state,
			}
			if j, err := json.Marshal(s); err != nil {
				io.WriteString(ws, fmt.Sprintf("{\"Type\": \"ERROR\", \"Message\": \"state json error: %v\"}", err))
				return
			} else {
				ws.Write(j)
			}

			// Push the move
			m := <-updateChan
			m.Type = "MOVE"
			if j, err := json.Marshal(m); err != nil {
				io.WriteString(ws, fmt.Sprintf("{\"Type\": \"ERROR\", \"Message\": \"move json error: %v\"}", err))
				return
			} else {
				ws.Write(j)
			}

		}
	}))

	http.HandleFunc("/drop", func(w http.ResponseWriter, r *http.Request) {
		stateMux.Lock()
		defer stateMux.Unlock()

		var d drop
		if !parseRequestJson(w, r, &d) {
			return
		}

		if d.PlayerId != state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, "{\"Message\": \"Not your turn!\"}")
			return
		}

		logs = append(logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Drop(d.Card); err != nil {
			w.WriteHeader(500)
			io.WriteString(w, fmt.Sprintf("Couldn't drop: %v", err))
			logs = append(logs, fmt.Sprintf("FAIL drop: %#v\n", d.Card))
			return
		}
		logs = append(logs, fmt.Sprintf("drop: %#v\n", d.Card))

		// Update all of the clients, that there is some new state.
		for _, u := range clients {
			u <- move{Drop: &d}
		}
	})

	http.HandleFunc("/take", func(w http.ResponseWriter, r *http.Request) {
		stateMux.Lock()
		defer stateMux.Unlock()

		var t take
		if !parseRequestJson(w, r, &t) {
			return
		}

		if t.PlayerId != state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, "{\"Message\": \"Not your turn!\"}")
			return
		}

		logs = append(logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Take(t.Card, t.Table); err != nil {
			w.WriteHeader(500)
			io.WriteString(w, fmt.Sprintf("Couldn't take: %v", err))
			logs = append(logs, fmt.Sprintf("FAIL take: %#v, %#v\n", t.Card, t.Table))
			return
		}

		logs = append(logs, fmt.Sprintf("take: %#v, %#v\n", t.Card, t.Table))

		// Update all of the clients, that there is some new state.
		for _, u := range clients {
			u <- move{Take: &t}
		}
	})

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
