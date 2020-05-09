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
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// Match contains all of the state for a server coordinating a scopa match.
// The zero value for Match is valid.
type Match struct {
	sync.Mutex
	state     scopa.Game
	ID        int64
	logs      []string
	gameStart chan struct{} // Channel is closed when the game has started.
	players   []player
}

// Reset zereos out all of the fields and sets a new match ID.
func (m *Match) Reset(id int64) {
	*m = Match{ID: id}
}

func (m *Match) addPlayer(matchID int64, nick string, sb scoreboard) (chan struct{}, error) {
	m.Lock()
	defer m.Unlock()

	if matchID == m.ID {
		for _, p := range m.players {
			if p.nick == nick {
				p.client = make(chan struct{}, 1000)
				return p.client, nil
			}
		}
	}

	// Keep track of the number of players that have "joined".
	// Give them a player id.
	if len(m.players) >= 2 {
		return nil, fmt.Errorf("match is full")
	}

	for _, p := range m.players {
		if p.nick == nick {
			return nil, fmt.Errorf("nickname %s is already taken", nick)
		}
	}

	updateChan := make(chan struct{}, 1000)
	m.players = append(m.players, player{updateChan, nick})

	if m.gameStart == nil {
		m.gameStart = make(chan struct{}, 0)
	}

	if len(m.players) == 2 {
		// Now that we have all of the players, check if these two have played before, and if yes, who goes
		// first?
		n := sb.nextPlayer(m.players[0].nick, m.players[1].nick)
		if m.players[0].nick != n {
			m.players[0], m.players[1] = m.players[1], m.players[0]
		}

		names := make([]string, 0)
		for _, p := range m.players {
			names = append(names, p.nick)
		}
		m.state = scopa.NewGame(names)
		close(m.gameStart) // Broadcast that the game is ready to start to all clients.
	}
	return updateChan, nil
}

func (m *Match) scorecardKey() string {
	s := make([]string, 0)
	for _, p := range m.players {
		s = append(s, p.nick)
	}
	sort.Strings(s)
	return strings.Join(s, "|")
}

type scoreboard map[string]*scorecard

type scorecard struct {
	Scores     map[string]int
	NextPlayer string
}

func scorekey(aNick, bNick string) string {
	n := []string{aNick, bNick}
	sort.Strings(n)
	return strings.Join(n, "|")
}

func (sb scoreboard) scores(aNick, bNick string) map[string]int {
	if v := sb[scorekey(aNick, bNick)]; v != nil {
		return v.Scores
	}
	return map[string]int{}
}

func (sb scoreboard) record(aNick, bNick string, aScore, bScore int) {
	key := scorekey(aNick, bNick)
	s, ok := sb[key]
	if !ok {
		// First time these two players have played eachother.
		// b goes first next time.
		sb[key] = &scorecard{
			Scores:     map[string]int{aNick: aScore, bNick: bScore},
			NextPlayer: bNick,
		}
		return
	}

	s.Scores[aNick] += aScore
	s.Scores[bNick] += bScore

	// Match has been recorded, swap the next player...
	if s.NextPlayer == aNick {
		s.NextPlayer = bNick
	} else {
		s.NextPlayer = aNick
	}
}

func (sb scoreboard) nextPlayer(aNick, bNick string) string {
	if v, ok := sb[scorekey(aNick, bNick)]; ok {
		return v.NextPlayer
	}
	return aNick
}

func (sb scoreboard) save(filename string) {
	b, err := json.Marshal(sb)
	if err != nil {
		fmt.Printf("Couldn't convert scoreboard to json: %v\n", err)
		return
	}

	if err := ioutil.WriteFile(filename, b, 0644); err != nil {
		fmt.Printf("Couldn't write to %s: %v\n", filename, err)
	}
}

func loadScoreboard(f string) scoreboard {
	sb := make(scoreboard)

	b, err := ioutil.ReadFile(f)
	if err != nil {
		fmt.Printf("Couldn't read %s, %v\n", f, err)
		return sb
	}

	if err := json.Unmarshal(b, &sb); err != nil {
		fmt.Printf("Couldn't parse json from %s, %v\n", f, err)
	}
	return sb
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

func (m *Match) endTurn(sb scoreboard) {
	m.logs = append(m.logs, fmt.Sprintf("state: %#v\n", m.state))

	if m.state.Ended() {
		// Record the scores.
		p1, p2 := m.state.Players[0], m.state.Players[1]
		a1, a2 := len(p1.Awards)+p1.Scopas, len(p2.Awards)+p2.Scopas
		n1, n2 := m.players[0].nick, m.players[1].nick
		sb.record(n1, n2, a1, a2)
	}

	// Update all of the clients, that there is some new state.
	for _, p := range m.players {
		var s struct{}
		p.client <- s
	}
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
