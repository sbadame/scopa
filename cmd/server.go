package main

import (
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/sbadame/scopa/autoreload"
	"github.com/sbadame/scopa/scopa"
	"golang.org/x/crypto/acme/autocert"
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
	port      = flag.Int("port", 8080, "The port to listen on for requests.")
	random    = flag.Bool("random", false, "When set to true, actually uses a random seed.")
	httpsHost = flag.String("https_host", "", "Set this to the hostname to get a Let's Encrypt SSL certifcate for.")

	// Populated at compile time with `go build/run -ldflags "-X main.gitCommit=$(git rev-parse HEAD)"`
	gitCommit string
)

// Wrap error messages into json so that javascript client code can always expect json.
func errorJson(message string) string {
	return "{\"Type\": \"ERROR\", \"Message\": \"" + message + "\"}"
}

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

// /join streams updates with JSON marshalled from this type to clients.
type update struct {
	Type  string
	State scopa.State
}

func parseRequestJson(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	c, err := strconv.Atoi(r.Header["Content-Length"][0])
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, errorJson(fmt.Sprintf("Couldn't get the content length: %v", err)))
		return false
	}

	buf := make([]byte, c)
	if _, err := io.ReadFull(r.Body, buf); err != nil {
		w.WriteHeader(500)
		io.WriteString(w, errorJson(fmt.Sprintf("Error reading data: %v", err)))
		return false
	}

	if err := json.Unmarshal(buf, &v); err != nil {
		w.WriteHeader(400)
		io.WriteString(w, errorJson(fmt.Sprintf("Error parsing json: %v", err)))
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
	var stateMux sync.Mutex
	gameId := time.Now().Unix()
	clients := make([]chan struct{}, 0)
	logs := make([]string, 0)

	// Serve resources for testing.
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// Reset the game...
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		state = scopa.NewGame()
		playerCount = 0
		gameId = time.Now().Unix()
		clients = make([]chan struct{}, 0)
		logs = make([]string, 0)
	})

	// Get Debug logs to repro
	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		if len(gitCommit) > 0 {
		    io.WriteString(w, fmt.Sprintf("Version: git checkout %s\n", gitCommit))
		} else {
		    io.WriteString(w, "Built with an unknown git version (-X main.gitCommit was not set)\n")
		}
		stateMux.Lock()
		defer stateMux.Unlock()
		for _, s := range logs {
			io.WriteString(w, s)
			io.WriteString(w, "\n")
		}
	})

	http.Handle("/join", websocket.Handler(func(ws *websocket.Conn) {
		var err error
		playerId := 0

		p := ws.Request().FormValue("PlayerId")
		g := ws.Request().FormValue("GameId")
		if p == "" || g == "" || g != strconv.FormatInt(gameId, 10) {
			if playerId, err = allocatePlayerId(); err != nil {
				ws.Close()
				return
			}
		} else {
			if playerId, err = strconv.Atoi(p); err != nil {
				ws.Close()
				return
			}
		}
		io.WriteString(ws, fmt.Sprintf("{\"Type\": \"INIT\", \"PlayerId\": %d, \"GameId\": %d}", playerId, gameId))

		// "Register" a channel to listen to changes to.
		updateChan := make(chan struct{}, 1000)
		clients = append(clients, updateChan)

		// Push the initial state, then keep pushing the full state with every change.
		for {
			// Push the game state
			s := update{
				Type:  "STATE",
				State: state,
			}
			if err := websocket.JSON.Send(ws, s); err != nil {
				io.WriteString(ws, errorJson(fmt.Sprintf("state json send error: %#v", err)))
				return
			}

			// Wait for an update...
			<-updateChan
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
			io.WriteString(w, errorJson("Not your turn!"))
			return
		}

		logs = append(logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Drop(d.Card); err != nil {
			switch err.(type) {
			case scopa.MoveError:
				w.WriteHeader(400)
			default:
				w.WriteHeader(500)
			}
			io.WriteString(w, errorJson(err.Error()))
			logs = append(logs, fmt.Sprintf("FAIL drop: %#v, %#v\n", d.Card, err))
			return
		}
		logs = append(logs, fmt.Sprintf("drop: %#v\n", d.Card))

		// Update all of the clients, that there is some new state.
		for _, u := range clients {
			var s struct{}
			u <- s
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
			io.WriteString(w, errorJson("Not your turn!"))
			return
		}

		logs = append(logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Take(t.Card, t.Table); err != nil {
			switch err.(type) {
			case scopa.MoveError:
				w.WriteHeader(400)
			default:
				w.WriteHeader(500)
			}
			io.WriteString(w, errorJson(err.Error()))
			logs = append(logs, fmt.Sprintf("FAIL take: %#v, %#v, %#v\n", t.Card, t.Table, err))
			return
		}

		logs = append(logs, fmt.Sprintf("take: %#v, %#v\n", t.Card, t.Table))

		// Update all of the clients, that there is some new state.
		for _, u := range clients {
			var s struct{}
			u <- s
		}
	})

	if *httpsHost != "" {
		// Still create an http server, but make it always redirect to https
		s := http.Server{
			Handler: http.RedirectHandler("https://"+*httpsHost, http.StatusMovedPermanently),
		}
		go func() { log.Fatal(s.ListenAndServe()) }()

		// The https server does all of the work and blocks until it's closed.
		log.Fatal(http.Serve(autocert.NewListener(*httpsHost), nil))
	} else {
		// Don't do any SSL stuff (useful for development)
		http.ListenAndServe(":"+strconv.Itoa(*port), nil)
	}
}
