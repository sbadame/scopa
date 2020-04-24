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
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	httpPort  = flag.Int("http_port", 8080, "The port to listen on for http requests.")
	random    = flag.Bool("random", false, "When set to true, actually uses a random seed.")
	httpsPort = flag.Int("https_port", 8081, "The port to listen on for https requests.")
	httpsHost = flag.String("https_host", "", "Set this to the hostname to get a Let's Encrypt SSL certificate for.")

	// Populated at compile time with `go build/run -ldflags "-X main.gitCommit=$(git rev-parse HEAD)"`
	gitCommit string
)

// Wrap error messages into json so that javascript client code can always expect json.
func errorJSON(message string) string {
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

type Game struct {
	sync.Mutex
	state       scopa.State
	gameId      int64
	clients     []chan struct{}
	logs        []string
	playerCount int
}

func NewGame() Game {
	return Game{
		sync.Mutex{},
		scopa.NewGame(),
		time.Now().Unix(),
		make([]chan struct{}, 0),
		make([]string, 0),
		0,
	}
}

// Keep track of the number of players that have "joined".
// Give them a player id.

func (g *Game) allocatePlayerId() (int, error) {
	g.Lock()
	defer g.Unlock()

	if g.playerCount >= 2 {
		return -1, fmt.Errorf("{\"Message\": \"Game is full!\"}")
	}

	g.playerCount++
	return g.playerCount, nil
}

func parseRequestJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	c, err := strconv.Atoi(r.Header["Content-Length"][0])
	if err != nil {
		w.WriteHeader(500)
		io.WriteString(w, errorJSON(fmt.Sprintf("Couldn't get the content length: %v", err)))
		return false
	}

	buf := make([]byte, c)
	if _, err := io.ReadFull(r.Body, buf); err != nil {
		w.WriteHeader(500)
		io.WriteString(w, errorJSON(fmt.Sprintf("Error reading data: %v", err)))
		return false
	}

	if err := json.Unmarshal(buf, &v); err != nil {
		w.WriteHeader(400)
		io.WriteString(w, errorJSON(fmt.Sprintf("Error parsing json: %v", err)))
		return false
	}
	return true
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\nBuilt at version: %s\n", os.Args[0], gitCommit)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *random {
		rand.Seed(time.Now().Unix())
	}

	game := NewGame()

	// Serve resources for testing.
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// Reset the game...
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		game = NewGame()
	})

	// Get Debug logs to repro
	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		if len(gitCommit) > 0 {
			io.WriteString(w, fmt.Sprintf("Version: git checkout %s\n", gitCommit))
		} else {
			io.WriteString(w, "Built with an unknown git version (-X main.gitCommit was not set)\n")
		}
		game.Lock()
		defer game.Unlock()
		for _, s := range game.logs {
			io.WriteString(w, s)
			io.WriteString(w, "\n")
		}
	})

	http.Handle("/join", websocket.Handler(func(ws *websocket.Conn) {
		var err error
		playerId := 0

		p := ws.Request().FormValue("PlayerId")
		g := ws.Request().FormValue("GameId")
		if p == "" || g == "" || g != strconv.FormatInt(game.gameId, 10) {
			if playerId, err = game.allocatePlayerId(); err != nil {
				ws.Close()
				return
			}
		} else {
			if playerId, err = strconv.Atoi(p); err != nil {
				ws.Close()
				return
			}
		}
		io.WriteString(ws, fmt.Sprintf("{\"Type\": \"INIT\", \"PlayerId\": %d, \"GameId\": %d}", playerId, game.gameId))

		// "Register" a channel to listen to changes to.
		updateChan := make(chan struct{}, 1000)
		game.clients = append(game.clients, updateChan)

		// Push the initial state, then keep pushing the full state with every change.
		for {
			// Push the game state
			s := update{
				Type:  "STATE",
				State: game.state,
			}
			if err := websocket.JSON.Send(ws, s); err != nil {
				io.WriteString(ws, errorJSON(fmt.Sprintf("state json send error: %#v", err)))
				return
			}

			// Wait for an update...
			<-updateChan
		}
	}))

	http.HandleFunc("/drop", func(w http.ResponseWriter, r *http.Request) {
		game.Lock()
		defer game.Unlock()

		var d drop
		if !parseRequestJSON(w, r, &d) {
			return
		}

		if d.PlayerId != game.state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, errorJSON("Not your turn!"))
			return
		}

		state := game.state
		game.logs = append(game.logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Drop(d.Card); err != nil {
			switch err.(type) {
			case scopa.MoveError:
				w.WriteHeader(400)
			default:
				w.WriteHeader(500)
			}
			io.WriteString(w, errorJSON(err.Error()))
			game.logs = append(game.logs, fmt.Sprintf("FAIL drop: %#v, %#v\n", d.Card, err))
			return
		}
		game.logs = append(game.logs, fmt.Sprintf("drop: %#v\n", d.Card))

		// Update all of the clients, that there is some new state.
		for _, u := range game.clients {
			var s struct{}
			u <- s
		}
	})

	http.HandleFunc("/take", func(w http.ResponseWriter, r *http.Request) {
		game.Lock()
		defer game.Unlock()

		var t take
		if !parseRequestJSON(w, r, &t) {
			return
		}

		if t.PlayerId != game.state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, errorJSON("Not your turn!"))
			return
		}

		state := game.state
		game.logs = append(game.logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Take(t.Card, t.Table); err != nil {
			switch err.(type) {
			case scopa.MoveError:
				w.WriteHeader(400)
			default:
				w.WriteHeader(500)
			}
			io.WriteString(w, errorJSON(err.Error()))
			game.logs = append(game.logs, fmt.Sprintf("FAIL take: %#v, %#v, %#v\n", t.Card, t.Table, err))
			return
		}

		game.logs = append(game.logs, fmt.Sprintf("take: %#v, %#v\n", t.Card, t.Table))

		// Update all of the clients, that there is some new state.
		for _, u := range game.clients {
			var s struct{}
			u <- s
		}
	})

	if *httpsHost != "" {
		// Still create an http server, but make it always redirect to https
		s := http.Server{
			Addr:    ":" + strconv.Itoa(*httpPort),
			Handler: http.RedirectHandler("https://"+*httpsHost, http.StatusMovedPermanently),
		}
		go func() { log.Fatal(s.ListenAndServe()) }()

		// To avoid the need to bind to 80/443 directly (and thus requiring the server to run as root)
		// we need to create our own autocert.Manager instead of using autocert.NewListener()
		m := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache("golang-autocert"),
			HostPolicy: autocert.HostWhitelist(*httpsHost),
		}
		ss := &http.Server{
			Addr:      ":" + strconv.Itoa(*httpsPort),
			TLSConfig: m.TLSConfig(),
		}
		// The https server does all of the work and blocks until it's closed.
		log.Fatal(ss.ListenAndServeTLS("", ""))
	} else {
		// Don't do any SSL stuff (useful for development)
		log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*httpPort), nil))
	}
}
