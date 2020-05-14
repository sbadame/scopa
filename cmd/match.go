package main

import (
	"encoding/json"
	"fmt"
	"github.com/sbadame/scopa/scopa"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

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
