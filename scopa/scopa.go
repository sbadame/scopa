package scopa

// TODO:
//  1. Forcing a place to make a trick if they can.
//  2. Support for 4 players?

import (
	"fmt"
	"math/rand"
)

type Suit int

const (
	UnknownSuit Suit = iota
	Denari
	Coppe
	Bastoni
	Spade
)

type Card struct {
	Suit Suit
	Value int
}

type Player struct {
	Id      int
	Hand    []Card
	Grabbed []Card
	Scopas  int
	Awards  []string
}

type drop struct {
	PlayerId int
	Card     Card
}

type take struct {
	PlayerId int
	Card     Card
	Table    []Card
}

type move struct {
	Drop *drop
	Take *take
}
type State struct {
	NextPlayer       int
	LastPlayerToTake int
	Deck             []Card
	Table            []Card
	Players          []Player
	LastMove         move
}

// An error for when the player is trying a move that is invalid.
type MoveError struct {
	message string
}

func (e MoveError) Error() string {
	return e.message
}

func moveErrorf(format string, a ...interface{}) error {
	return &MoveError{fmt.Sprintf(format, a...)}
}

func badMathTake(card Card, take []Card) error {
	return moveErrorf("%v can't take %v", card, take)
}

func cardMissingFromTable(card Card, table []Card) error {
	return moveErrorf("%v is not a card on the table %v", card, table)
}

func perroError(card Card) error {
	return moveErrorf("You gotta take %v", card)
}

func NewDeck() []Card {

	// Construct a full deck of cards.
	d := make([]Card, 40)

	i := 0
	for s := 1; s <= 4; s++ {
		for v := 1; v <= 10; v++ {
			d[i] = Card{Suit(s), v}
			i += 1
		}
	}
	return d
}

func Shuffle(cards []Card) {
	rand.Shuffle(len(cards), func(i, j int) {
		cards[i], cards[j] = cards[j], cards[i]
	})
}

func NewGame() State {
	// Create the game state with no cards
	s := State{
		NextPlayer: 1,
		Players:    []Player{Player{Id: 1}, Player{Id: 2}},
	}
	p1, p2 := &s.Players[0], &s.Players[1]

	// Now lets deal out the cards
	cards := NewDeck()

	// Keep shuffling until we don't see more than 2 Re's on the table (first 4 cards)
	for {
		Shuffle(cards)

		r := 0
		for i := 0; i < 4; i++ {
			if cards[i].Value == 10 {
				r += 1
			}
		}
		if r <= 2 {
			break
		}
	}

	// This is not the standard dealing order...  Oh well...
	// 4 on the table, 3 cards to each player, rest go into the Game's deck.
	s.Table = append(s.Table, cards[:4]...)
	p1.Hand = append(p1.Hand, cards[4:7]...)
	p2.Hand = append(p2.Hand, cards[7:10]...)
	s.Deck = append(s.Deck, cards[10:]...)

	return s
}

func (s *State) Check() error {
	// Check that there are 40 cards
	c := 0
	c += len(s.Deck)
	c += len(s.Table)
	for _, p := range s.Players {
		c += len(p.Hand)
		c += len(p.Grabbed)
	}
	if c != 40 {
		return fmt.Errorf("Failed 40 card check, found %d", c)
	}

	// Check that no card appears more than once.
	m := make(map[Card]bool)
	// Deck
	for _, i := range s.Deck {
		if m[i] {
			return fmt.Errorf("Deck has repeated cards, found %#v, \n%#v", i, s)
		}
		m[i] = true
	}

	// Table
	for _, i := range s.Table {
		if m[i] {
			return fmt.Errorf("Deck has repeated cards, found %#v, \n%#v", i, s)
		}
		m[i] = true
	}

	// Players
	for _, p := range s.Players {
		for _, c := range p.Hand {
			if m[c] {
				return fmt.Errorf("Deck has repeated cards, found %#v, \n%#v", c, s)
			}
			m[c] = true
		}
		for _, c := range p.Grabbed {
			if m[c] {
				return fmt.Errorf("Deck has repeated cards, found %#v, \n%#v", c, s)
			}
			m[c] = true
		}
	}

	// Check that player values make sense
	maxPlayers := len(s.Players)
	for _, p := range s.Players {
		if p.Id <= 0 || p.Id > maxPlayers {
			return fmt.Errorf("Player Id is invalid")
		}
	}
	return nil
}

func Index(c Card, s []Card) int {
	for i, x := range s {
		if x == c {
			return i
		}
	}
	return -1
}

func Contains(c Card, s []Card) bool {
	return Index(c, s) != -1
}

func Remove(i int, s []Card) []Card {
	return append(s[:i], s[i+1:]...)
}

func RemoveCard(c Card, s []Card) ([]Card, error) {
	if i := Index(c, s); i == -1 {
		return nil, fmt.Errorf("Didn't find %v in %v", c, s)
	} else {
		return Remove(i, s), nil
	}
}

func (s State) CheckCurrentPlayerHasCard(card Card) error {
	// Check that the player actually has the card.
	playerId := s.NextPlayer - 1
	p := &s.Players[playerId]
	var handIndex int
	if handIndex = Index(card, p.Hand); handIndex == -1 {
		return fmt.Errorf("Player %d doesn't have %v in their hand: %v", s.NextPlayer, card, p.Hand)
	}
	return nil
}

func (s State) EmptyHands() bool {
	// Check whether we need to deal out more cards...
	for _, p := range s.Players {
		if len(p.Hand) > 0 {
			return false
		}
	}
	return true
}

func MostCards(p1, p2 *Player) {
	if len(p1.Grabbed) == len(p2.Grabbed) {
		return
	}
	if len(p1.Grabbed) > len(p2.Grabbed) {
		p1.Awards = append(p1.Awards, "Cards")
	} else {
		p2.Awards = append(p2.Awards, "Cards")
	}
}

func MostDenari(p1, p2 *Player) {
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

func SetteBello(p1, p2 *Player) {
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

func Primera(p1, p2 *Player) {
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

func (s *State) EndTurn() error {
	// Move the turn to the next player.
	s.NextPlayer += 1
	if s.NextPlayer > len(s.Players) {
		s.NextPlayer = 1
	}

	// Check if this was actually the end of the game...
	if len(s.Deck) == 0 && s.EmptyHands() {

		// Give the player that last took cards, the remaining cards on the table.
		g := &s.Players[s.LastPlayerToTake-1]
		g.Grabbed = append(g.Grabbed, s.Table...)

		// Not that it matters, but remove the last cards from the table.
		s.Table = []Card{}

		// Count points
		MostCards(&s.Players[0], &s.Players[1])
		MostDenari(&s.Players[0], &s.Players[1])
		SetteBello(&s.Players[0], &s.Players[1])
		Primera(&s.Players[0], &s.Players[1])
		return nil
	}

	if s.EmptyHands() {
		// Deal out the next 3 cards to each player and remove them from the deck.
		s.Players[0].Hand = s.Deck[:3]
		s.Players[1].Hand = s.Deck[3:6]
		s.Deck = s.Deck[6:]
	}

	return nil
}

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
		if !Contains(t, s.Table) {
			return cardMissingFromTable(t, s.Table)
		}
	}

	if err := s.CheckCurrentPlayerHasCard(card); err != nil {
		return err
	}

	// Take the Face
	v := card.Value
	if (v > 7) && (len(table) > 1) {
		// Check if there is a face match
		for _, t := range s.Table {
			// If there card in your hand direct equals a card in the pot and you're trying to take > 1.... no no no
			if (v == t.Value) && (!Contains(t, table)) {
				return perroError(t)
			}
		}
	}

	// Looking good! Lets do the move!
	playerId := s.NextPlayer - 1
	p := &s.Players[playerId]

	// Remove the card from the player's hand.
	if newHand, err := RemoveCard(card, p.Hand); err != nil {
		return err
	} else {
		p.Hand = newHand
	}

	// Put it into the player's grabbed pile.
	p.Grabbed = append(p.Grabbed, card)

	// Remove the cards from the table.
	for _, t := range table {
		if newTable, err := RemoveCard(t, s.Table); err != nil {
			return err
		} else {
			s.Table = newTable
		}
	}

	// Add them to the player's pile
	p.Grabbed = append(p.Grabbed, table...)

	// Check if that was a scopa...
	if len(s.Table) == 0 {
		p.Scopas += 1
	}

	s.LastPlayerToTake = p.Id
	s.LastMove = move{Take: &take{s.NextPlayer, card, table}}
	return s.EndTurn()
}

func (s *State) Drop(card Card) error {
	// Validating inputs...
	if err := s.CheckCurrentPlayerHasCard(card); err != nil {
		return err
	}

	// Looks good, drop the card on the table.
	playerId := s.NextPlayer - 1
	p := &s.Players[playerId]

	// Remove the card from the player's hand.
	if newHand, err := RemoveCard(card, p.Hand); err != nil {
		return err
	} else {
		p.Hand = newHand
	}

	// Add the card to the table
	s.Table = append(s.Table, card)
	s.LastMove = move{Drop: &drop{s.NextPlayer, card}}

	return s.EndTurn()
}
