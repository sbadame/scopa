package main

import (
	"encoding/json"
	"fmt"
	"github.com/sbadame/scopa/scopa"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
	"sync"
)

// To test with no random seed:
// $ curl -v localhost:8080/join
// $ curl -v localhost:8080/state
// $ curl -v localhost:8080/take -H "Content-Type: application/json" --data '{"PlayerId": 1, "Card": {"Suite":4, "Value":2}, "Table": [{"Suite": 1, "Value": 2}]}'
// $ curl -v localhost:8080/drop -H "Content-Type: application/json" --data '{"Card": {"Suite":4, "Value":2}}'

type drop struct {
	PlayerId int
	Card scopa.Card
}

type take struct {
	PlayerId int
	Card  scopa.Card
	Table []scopa.Card
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

func main() {
	state := scopa.NewGame()
	gameId := time.Now().Unix()

	// Keep track of the number of players that have "joined".
	// Give them a player id.
	var playerCount int
	var playerMux sync.Mutex

	// Serve resources for testing.
	http.Handle("/", http.FileServer(http.Dir("./web")))

	http.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		playerMux.Lock()
		defer playerMux.Unlock()
		if playerCount >= 2 {
			w.WriteHeader(400)
			io.WriteString(w, fmt.Sprintf("{\"Message\": \"Game is full!\"}"))
			return
		}
		io.WriteString(w, fmt.Sprintf("{\"PlayerId\": %d, \"GameId\": %d}", playerCount + 1, gameId))
		playerCount += 1
	})

	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		if j, err := json.Marshal(state); err != nil {
			w.WriteHeader(500)
			io.WriteString(w, fmt.Sprintf("Error encoding json: %v", err))
		} else {
			w.Write(j)
		}
	})

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
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
