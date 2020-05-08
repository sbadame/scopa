package scopa

// TODO:
//  1. Forcing a place to make a trick if they can.
//  2. Support for 4 players?

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
)

// Suit of a card.
type Suit string

// The different Card Suits.
const (
	UnknownSuit Suit = "[Unknown]"
	Denari           = "Denari"
	Coppe            = "Coppe"
	Bastoni          = "Bastoni"
	Spade            = "Spade"
)

// Card is a Card that is in the game.
type Card struct {
	Suit  Suit
	Value int
}

// String is the human readable representation of the card.
func (c Card) String() string {
	if c.Suit == Denari && c.Value == 7 {
		return "Settebello"
	}

	v := strconv.Itoa(c.Value)
	switch v {
	case "8":
		v = "Fante"
	case "9":
		v = "Cavallo"
	case "10":
		v = "Re"
	}
	return fmt.Sprintf("%s di %s", v, c.Suit)
}

// MarshalJSON customizes the Card JSON representation to include a "Name" field.
func (c Card) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Suit  Suit
		Value int
		Name  string
	}{
		c.Suit,
		c.Value,
		c.String(),
	})
}

// Player is particpant in the Scopa game.
type Player struct {
	Name    string
	Hand    []Card
	Grabbed []Card
	Scopas  int
	Awards  []string
}

func (p Player) holds(card Card) error {
	for _, c := range p.Hand {
		if c == card {
			return nil
		}
	}
	return fmt.Errorf("Player %s doesn't have %s in their hand: %v", p.Name, card, p.Hand)
}

type drop struct {
	Player string
	Card   Card
}

type take struct {
	Player string
	Card   Card
	Table  []Card
}

type move struct {
	Drop *drop
	Take *take
}

type State struct {
	NextPlayer       string
	LastPlayerToTake string
	Deck             []Card
	Table            []Card
	Players          []Player
	LastMove         move
}

// JSONForPlayer customizes the JSON output to include a mapping of player name to Player.
func (s *State) JSONForPlayer(name string) ([]byte, error) {
	p, err := s.player(name)
	if err != nil {
		return nil, err
	}
	j := struct {
		NextPlayer       string
		LastPlayerToTake string
		Table            []Card
		Players          []Player
		Player           Player
		LastMove         move
		Ended            bool
	}{
		s.NextPlayer,
		s.LastPlayerToTake,
		s.Table,
		make([]Player, 0),
		*p,
		s.LastMove,
		s.Ended(),
	}
	for _, p := range s.Players {
		j.Players = append(j.Players, p)
	}
	return json.Marshal(j)
}

func (s *State) player(name string) (*Player, error) {
	for i, p := range s.Players {
		if p.Name == name {
			return &s.Players[i], nil
		}
	}
	return nil, fmt.Errorf("%s isn't a player in %v", name, s.Players)
}

func (s *State) nextPlayer() *Player {
	for i, p := range s.Players {
		if p.Name == s.NextPlayer {
			return &s.Players[(i+1)%len(s.Players)]
		}
	}
	panic(fmt.Sprintf("s.NextPlayer (%s) was not found in %v", s.NextPlayer, s.Players))
}

func (s *State) currentPlayer() *Player {
	for i, p := range s.Players {
		if p.Name == s.NextPlayer {
			return &s.Players[i]
		}
	}
	panic(fmt.Sprintf("s.NextPlayer (%s) was not found in %v", s.NextPlayer, s.Players))
}

// MoveError is used when the player is attempting an invalid move.
type MoveError struct {
	Message string
}

func (e MoveError) Error() string {
	return e.Message
}

func moveErrorf(format string, a ...interface{}) error {
	return &MoveError{fmt.Sprintf(format, a...)}
}

func badMathTake(card Card, take []Card) error {
	return moveErrorf("%s can't take %s", card, take)
}

func cardMissingFromTable(card Card, table []Card) error {
	return moveErrorf("%s is not a card on the table %s", card, table)
}

func perroError(card Card) error {
	return moveErrorf("You gotta take %s", card)
}

// NewDeck creates a newly shuffled deck.
func NewDeck() []Card {

	// Construct a full deck of cards.
	d := make([]Card, 0)
	for _, s := range []Suit{Denari, Coppe, Bastoni, Spade} {
		for v := 1; v <= 10; v++ {
			d = append(d, Card{s, v})
		}
	}

	rand.Shuffle(len(d), func(i, j int) {
		d[i], d[j] = d[j], d[i]
	})
	return d
}

// NewGame creates a game with the given names as player names.
// They will play in the order provided.
func NewGame(names []string) State {
	// Create the game state with no cards
	s := State{NextPlayer: names[0]}
	for _, n := range names {
		s.Players = append(s.Players, Player{Name: n})
	}

	// Keep shuffling and dealing until we don't see more than 2 Re's on the table
	for {
		cards := NewDeck()

		// Moves 1 card from one slice to another.
		deal := func(to *[]Card) {
			*to = append(*to, cards[0])
			cards = cards[1:]
		}
		deal(&s.Table)

		// Round robin 3 cards to each player, rest go into the Game's deck.
		for x := 0; x < 3; x++ {
			for i := range s.Players {
				deal(&s.Players[i].Hand)
			}
			deal(&s.Table)
		}

		// Check if there are 2 or more Re's on the table.
		r := 0
		for i := 0; i < 4; i++ {
			if cards[i].Value == 10 {
				r++
			}
		}
		if r <= 2 {
			// Good deal.
			s.Deck = cards
			break
		}
	}
	return s
}

func index(c Card, s []Card) int {
	for i, x := range s {
		if x == c {
			return i
		}
	}
	return -1
}

func contains(c Card, s []Card) bool {
	return index(c, s) != -1
}

func removeCard(c Card, s []Card) ([]Card, error) {
	i := index(c, s)
	if i == -1 {
		return nil, fmt.Errorf("%v coesn't contain %v", s, c)
	}

	// Remove the card
	return append(s[:i], s[i+1:]...), nil
}

func (s State) emptyHands() bool {
	// Check whether we need to deal out more cards...
	for _, p := range s.Players {
		if len(p.Hand) > 0 {
			return false
		}
	}
	return true
}

func mostCards(p1, p2 *Player) {
	if len(p1.Grabbed) == len(p2.Grabbed) {
		return
	}
	if len(p1.Grabbed) > len(p2.Grabbed) {
		p1.Awards = append(p1.Awards, "Cards")
	} else {
		p2.Awards = append(p2.Awards, "Cards")
	}
}

func mostDenari(p1, p2 *Player) {
	var a int
	for _, c := range p1.Grabbed {
		if c.Suit == Denari {
			a += 1
		}
	}

	var b int
	for _, c := range p2.Grabbed {
		if c.Suit == Denari {
			b += 1
		}
	}

	if a == b {
		return
	}

	if a > b {
		p1.Awards = append(p1.Awards, "Denari")
	} else {
		p2.Awards = append(p2.Awards, "Denari")
	}
}

func setteBello(p1, p2 *Player) {
	setteBello := Card{Denari, 7}
	for _, c := range p1.Grabbed {
		if c == setteBello {
			p1.Awards = append(p1.Awards, "SetteBello")
			return
		}
	}
	p2.Awards = append(p2.Awards, "SetteBello")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func playerPrimera(p Player) int {

	points := map[int]int{
		7:  21,
		6:  18,
		1:  16,
		5:  15,
		4:  14,
		3:  13,
		2:  12,
		10: 10,
	}

	var d, s, k, b int
	for _, c := range p.Grabbed {
		if c.Suit == Denari {
			d = max(d, points[c.Value])
		}
		if c.Suit == Spade {
			s = max(s, points[c.Value])
		}
		if c.Suit == Coppe {
			k = max(k, points[c.Value])
		}
		if c.Suit == Bastoni {
			b = max(b, points[c.Value])
		}
	}

	return d + s + k + b
}

func primera(p1, p2 *Player) {
	a, b := playerPrimera(*p1), playerPrimera(*p2)

	if a == b {
		return
	}

	if a > b {
		p1.Awards = append(p1.Awards, "Primera")
	} else {
		p2.Awards = append(p2.Awards, "Primera")
	}
}

func (s *State) endTurn() error {
	// Move the turn to the next player.
	s.NextPlayer = s.nextPlayer().Name

	if s.Ended() {
		// Give the player that last took cards, the remaining cards on the table.

		if p, err := s.player(s.LastPlayerToTake); err != nil {
			(*p).Grabbed = append((*p).Grabbed, s.Table...)
			panic(fmt.Errorf(`s.LastPlayerToTake is not in the list of players: %v`, err))
		}

		// Not that it matters, but remove the last cards from the table.
		s.Table = []Card{}

		// Count points
		mostCards(&s.Players[0], &s.Players[1])
		mostDenari(&s.Players[0], &s.Players[1])
		setteBello(&s.Players[0], &s.Players[1])
		primera(&s.Players[0], &s.Players[1])
		return nil
	}

	if s.emptyHands() {
		// Deal out the next 3 cards to each player and remove them from the deck.
		s.Players[0].Hand = s.Deck[:3]
		s.Players[1].Hand = s.Deck[3:6]
		s.Deck = s.Deck[6:]
	}

	return nil
}

// Take performs a trick where the current place takes cards from the table whos values add up to a card in their hand.
func (s *State) Take(card Card, table []Card) error {
	// Validating inputs...
	// Check that the math works out...
	sum := 0
	for _, t := range table {
		sum += t.Value
	}
	if sum != card.Value {
		return badMathTake(card, table)
	}

	// Check that the cards are actually on the table.
	for _, t := range table {
		if !contains(t, s.Table) {
			return cardMissingFromTable(t, s.Table)
		}
	}

	if err := s.currentPlayer().holds(card); err != nil {
		return err
	}

	// Take the Face
	v := card.Value
	if (v > 7) && (len(table) > 1) {
		// Check if there is a face match
		for _, t := range s.Table {
			// If there card in your hand direct equals a card in the pot and you're trying to take > 1.... no no no
			if (v == t.Value) && (!contains(t, table)) {
				return perroError(t)
			}
		}
	}

	// Looking good! Lets do the move!
	p := s.currentPlayer()

	// Remove the card from the player's hand.
	newHand, err := removeCard(card, p.Hand)
	if err != nil {
		return err
	}

	p.Hand = newHand

	// Put it into the player's grabbed pile.
	p.Grabbed = append(p.Grabbed, card)

	// Remove the cards from the table.
	for _, t := range table {
		newTable, err := removeCard(t, s.Table)
		if err != nil {
			return err
		}
		s.Table = newTable
	}

	// Add them to the player's pile
	p.Grabbed = append(p.Grabbed, table...)

	// Check if that was a scopa...
	if len(s.Table) == 0 {
		p.Scopas++
	}

	s.LastPlayerToTake = s.currentPlayer().Name
	s.LastMove = move{Take: &take{s.NextPlayer, card, table}}
	return s.endTurn()
}

// Drop performs a trick where the current player drops a card from their hand onto the table.
func (s *State) Drop(card Card) error {
	// Validating inputs...
	if err := s.currentPlayer().holds(card); err != nil {
		return err
	}

	// Looks good, drop the card on the table.
	p := s.currentPlayer()

	// Remove the card from the player's hand.
	newHand, err := removeCard(card, p.Hand)
	if err != nil {
		return err
	}

	p.Hand = newHand

	// Add the card to the table
	s.Table = append(s.Table, card)
	s.LastMove = move{Drop: &drop{s.NextPlayer, card}}

	return s.endTurn()
}

// Ended is true if the game has ended and there are no more moves.
func (s State) Ended() bool {
	if len(s.Deck) > 0 {
		return false
	}
	return s.emptyHands()
}
