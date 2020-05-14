package main

import (
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
	"time"
)

var (
	httpPort       = flag.Int("http_port", 8080, "The port to listen on for http requests.")
	random         = flag.Bool("random", false, "When set to true, actually uses a random seed.")
	httpsPort      = flag.Int("https_port", 8081, "The port to listen on for https requests.")
	httpsHost      = flag.String("https_host", "", "Set this to the hostname to get a Let's Encrypt SSL certificate for.")
	scoreboardFile = flag.String("scoreboard_file", "scoreboard.json", "The file to read and write scopa scores to.")

	// Populated at compile time with `go build/run -ldflags "-X main.gitCommit=$(git rev-parse HEAD)"`
	gitCommit string
)

// Wrap error messages into json so that javascript client code can always expect json.
func errorJSON(message string) string {
	return `{"Message": "` + message + `"}`
}

// /drop request content body json is marsheled into this struct.
type drop struct {
	Player string
	Card   scopa.Card
}

// /take request content body json is marshaled into this struct.
type take struct {
	Player string
	Card   scopa.Card
	Table  []scopa.Card
}

type player struct {
	client chan struct{}
	nick   string
}

type server struct {
	m  Match
	sb scoreboard
}

func (s *server) debug(w http.ResponseWriter, r *http.Request) {
	if len(gitCommit) > 0 {
		io.WriteString(w, fmt.Sprintf("Version: git checkout %s\n", gitCommit))
	} else {
		io.WriteString(w, "Built with an unknown git version (-X main.gitCommit was not set)\n")
	}

	s.m.Lock()
	defer s.m.Unlock()

	io.WriteString(w, fmt.Sprintf("MatchID: %d\n", s.m.ID))
	io.WriteString(w, fmt.Sprintf("Players: %#v\n", s.m.players))
	for _, n := range s.m.logs {
		io.WriteString(w, n)
		io.WriteString(w, "\n")
	}
}

func (s *server) join(ws *websocket.Conn) {
	match := &(s.m)
	errorf := func(format string, a ...interface{}) {
		io.WriteString(ws, errorJSON(fmt.Sprintf(format, a...)))
		ws.Close()
	}

	var err error

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

	updateChan, err := match.addPlayer(matchID, nick, s.sb)
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
		io.WriteString(ws, errorJSON("Failed to send the MatchID message."))
		return
	}

	// Block until all players have joined and the game is ready to start.
	<-match.gameStart

	init := struct {
		Nicknames map[int]string
		Scorecard map[string]int
	}{
		make(map[int]string),
		s.sb.scores(match.players[0].nick, match.players[1].nick),
	}
	for i, p := range match.players {
		init.Nicknames[i+1] = p.nick
	}
	if err := websocket.JSON.Send(ws, init); err != nil {
		io.WriteString(ws, errorJSON("Failed to send the Nicknames/Scorecard messages."))
		return
	}

	// Push the initial state, then keep pushing the full state with every change.
	for {
		// Push the match state with nick's and redacted info.
		if b, err := match.state.JSONForPlayer(nick); err == nil {
			io.WriteString(ws, fmt.Sprintf(`{"State": %s}`, b))
		} else {
			io.WriteString(ws, errorJSON(fmt.Sprintf("state json send error: %#v", err)))
			return
		}

		// Wait for an update...
		<-updateChan
	}
}

func (s *server) drop(w http.ResponseWriter, r *http.Request) {
	match := &(s.m)
	match.Lock()
	defer match.Unlock()

	var d drop
	if !parseRequestJSON(w, r, &d) {
		return
	}

	if d.Player != match.state.NextPlayer {
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
	match.endTurn(s.sb)
	s.sb.save(*scoreboardFile)
}

// Reset the match, no qustions asked, power users only...
func (s *server) reset(w http.ResponseWriter, r *http.Request) {
	s.m.Reset(time.Now().Unix())
}

// This will create a new match if one hasn't already been created.
func (s *server) newMatch(w http.ResponseWriter, r *http.Request) {
	match := &(s.m)
	p := struct{ OldMatchID int64 }{}
	if !parseRequestJSON(w, r, &p) {
		return
	}

	match.Lock()
	defer match.Unlock()

	if p.OldMatchID == match.ID {
		match.Reset(time.Now().Unix())
	}
}

func (s *server) matchID(w http.ResponseWriter, r *http.Request) {
	match := &(s.m)
	match.Lock()
	defer match.Unlock()
	io.WriteString(w, fmt.Sprintf(`{"MatchID": %d}`, match.ID))
}

func (s *server) take(w http.ResponseWriter, r *http.Request) {
	match := &(s.m)
	match.Lock()
	defer match.Unlock()

	var t take
	if !parseRequestJSON(w, r, &t) {
		return
	}

	if t.Player != match.state.NextPlayer {
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
	match.endTurn(s.sb)
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

	s := server{
		m:  Match{ID: time.Now().Unix()},
		sb: loadScoreboard(*scoreboardFile),
	}

	// Serve resources.
	http.Handle("/", http.FileServer(http.Dir("./web")))
	http.Handle("/join", websocket.Handler(s.join))
	http.HandleFunc("/debug", s.debug)
	http.HandleFunc("/drop", s.drop)
	http.HandleFunc("/take", s.take)
	http.HandleFunc("/matchID", s.matchID)
	http.HandleFunc("/newMatch", s.newMatch)
	http.HandleFunc("/reset", s.reset)

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
