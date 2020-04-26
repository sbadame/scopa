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
	return `{"Message": "` + message + `"}`
}

// /drop request content body json is marsheled into this struct.
type drop struct {
	PlayerID int
	Card     scopa.Card
}

// /take request content body json is marsheled into this struct.
type take struct {
	PlayerID int
	Card     scopa.Card
	Table    []scopa.Card
}

// /join streams updates with JSON marshalled from this type to clients.
type update struct {
	State scopa.State
}

type player struct {
	client chan struct{}
	nick   string
}

// Match contains all of the state for a server coordinating a scopa match.
type Match struct {
	sync.Mutex
	state     scopa.State
	ID        int64
	logs      []string
	gameStart chan struct{} // Channel is closed when the game has started.
	players   []player
}

func newMatch() Match {
	m := Match{
		sync.Mutex{},
		scopa.NewGame(),
		time.Now().Unix(),
		make([]string, 0),
		make(chan struct{}, 0),
		make([]player, 0),
	}
	m.logs = append(m.logs, fmt.Sprintf("state: %#v\n", m.state))
	return m
}

func (m *Match) join(playerID int, matchID int64, nick string) (int, chan struct{}, error) {
	m.Lock()
	defer m.Unlock()

	// If the matchIDs are equal, assume that someone is rejoining.
	if playerID != 0 && matchID == m.ID {
		c := make(chan struct{}, 1000)
		m.players[playerID-1].client = c
		return playerID, c, nil
	}

	// Keep track of the number of players that have "joined".
	// Give them a player id.
	if len(m.players) >= 2 {
		return -1, nil, fmt.Errorf("match is full")
	}

	for _, p := range m.players {
		if p.nick == nick {
			return -1, nil, fmt.Errorf("nickname %s is already taken", nick)
		}
	}

	updateChan := make(chan struct{}, 1000)
	m.players = append(m.players, player{updateChan, nick})
	playerID = len(m.players)

	if len(m.players) == 2 {
		close(m.gameStart) // Broadcast that the game is ready to start to all clients.
	}

	return playerID, updateChan, nil
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

	match := newMatch()

	// Serve resources for testing.
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// Reset the match...
	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		match = newMatch()
	})

	// Get Debug logs to repro
	http.HandleFunc("/debug", func(w http.ResponseWriter, r *http.Request) {
		if len(gitCommit) > 0 {
			io.WriteString(w, fmt.Sprintf("Version: git checkout %s\n", gitCommit))
		} else {
			io.WriteString(w, "Built with an unknown git version (-X main.gitCommit was not set)\n")
		}
		match.Lock()
		defer match.Unlock()

		io.WriteString(w, fmt.Sprintf("MatchID: %d\n", match.ID))
		io.WriteString(w, fmt.Sprintf("Players: %#v\n", match.players))
		for _, s := range match.logs {
			io.WriteString(w, s)
			io.WriteString(w, "\n")
		}
	})

	http.Handle("/join", websocket.Handler(func(ws *websocket.Conn) {

		errorf := func(format string, a ...interface{}) {
			io.WriteString(ws, errorJSON(fmt.Sprintf(format, a...)))
			ws.Close()
		}

		var err error

		playerID := 0
		if pid := ws.Request().FormValue("PlayerID"); pid != "" && pid != "null" && pid != "undefined" {
			if playerID, err = strconv.Atoi(pid); err != nil {
				errorf("PlayerID has an invalid value: %s", err)
				return
			}
		}

		matchID := int64(0)
		if mid := ws.Request().FormValue("MatchID"); mid != "" && mid != "null" && mid != "undefined" {
			if matchID, err = strconv.ParseInt(mid, 10, 64); err != nil {
				errorf("MatchID has an invalid value: %s", err)
				return
			}
		}

		nick := ws.Request().FormValue("Nickname")
		if nick == "" {
			errorf("Nickname field needs to be set.")
			return
		}

		playerID, updateChan, err := match.join(playerID, matchID, nick)
		if err != nil {
			errorf("%s", err)
			return
		}

		m := struct {
			MatchID int64
		}{
			match.ID,
		}
		if err := websocket.JSON.Send(ws, m); err != nil {
			io.WriteString(ws, errorJSON("Failed to send the MATCHID message."))
			return
		}

		// Block until all players have joined and the game is ready to start.
		<-match.gameStart

		init := struct {
			PlayerID  int
			Nicknames map[int]string
		}{
			playerID,
			make(map[int]string),
		}
		for i, p := range match.players {
			init.Nicknames[i+1] = p.nick
		}
		if err := websocket.JSON.Send(ws, init); err != nil {
			io.WriteString(ws, errorJSON("Failed to send the INIT message."))
			return
		}

		// Push the initial state, then keep pushing the full state with every change.
		for {
			// Push the match state
			s := update{
				State: match.state,
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
		match.Lock()
		defer match.Unlock()

		var d drop
		if !parseRequestJSON(w, r, &d) {
			return
		}

		if d.PlayerID != match.state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, errorJSON("Not your turn!"))
			return
		}

		state := &match.state
		match.logs = append(match.logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Drop(d.Card); err != nil {
			switch err.(type) {
			case scopa.MoveError:
				w.WriteHeader(400)
			default:
				w.WriteHeader(500)
			}
			io.WriteString(w, errorJSON(err.Error()))
			match.logs = append(match.logs, fmt.Sprintf("FAIL drop: %#v, %#v\n", d.Card, err))
			return
		}
		match.logs = append(match.logs, fmt.Sprintf("drop: %#v\n", d.Card))
		match.logs = append(match.logs, fmt.Sprintf("state: %#v\n", match.state))

		// Update all of the clients, that there is some new state.
		for _, p := range match.players {
			var s struct{}
			p.client <- s
		}
	})

	http.HandleFunc("/take", func(w http.ResponseWriter, r *http.Request) {
		match.Lock()
		defer match.Unlock()

		var t take
		if !parseRequestJSON(w, r, &t) {
			return
		}

		if t.PlayerID != match.state.NextPlayer {
			w.WriteHeader(400)
			io.WriteString(w, errorJSON("Not your turn!"))
			return
		}

		state := &match.state
		match.logs = append(match.logs, fmt.Sprintf("state: %#v\n", state))
		if err := state.Take(t.Card, t.Table); err != nil {
			switch err.(type) {
			case scopa.MoveError:
				w.WriteHeader(400)
			default:
				w.WriteHeader(500)
			}
			io.WriteString(w, errorJSON(err.Error()))
			match.logs = append(match.logs, fmt.Sprintf("FAIL take: %#v, %#v, %#v\n", t.Card, t.Table, err))
			match.logs = append(match.logs, fmt.Sprintf("state: %#v\n", match.state))
			return
		}

		match.logs = append(match.logs, fmt.Sprintf("take: %#v, %#v\n", t.Card, t.Table))
		match.logs = append(match.logs, fmt.Sprintf("state: %#v\n", match.state))

		// Update all of the clients, that there is some new state.
		for _, p := range match.players {
			var s struct{}
			p.client <- s
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
