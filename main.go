package main

// TODO:
//  1. Re-shuffling if the table has 3-4 Kings on the initial deal.
//  2. Forcing a place to make a trick if they can.
//  3. Forcing that only captures a single card and not multiple cards.
//  4. Support for 4 players?

import (
  "fmt"
  "math/rand"
  "encoding/json"
  "os"
)

type moveError struct {
  message string
}

func (e *moveError) Error() string {
  return e.message
}


type Suite int
const (
  UnknownSuite Suite = iota
  Denari
  Spade
  Coppe
  Bastoni
)

type Card struct {
  Suite Suite
  Value int
}

type Player struct {
  Id int
  Hand []Card
  Grabbed []Card
  Scopas int
  Awards []string
}

type State struct {
  NextPlayer int
  LastPlayerToTake int
  Deck []Card
  Table []Card
  Players []Player
}

func NewDeck() []Card {

  // Construct a full deck of cards.
  var d []Card
  for s := 1; s <= 4; s++ {
    for v := 1; v <= 10; v++ {
      d = append(d, Card{Suite(s), v})
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
  deck := NewDeck()

  Shuffle(deck)

  // This is not the standard dealing order...
  // Oh well...
  // Players first
  p1 := Player {
    Id: 1,
    Hand: deck[:3],
  }
  p2 := Player {
    Id: 2,
    Hand: deck[3:6],
  }
  deck = deck[6:]

  // Then the table
  table := deck[:4]
  deck = deck[4:]

  return State{
    NextPlayer: 1,
    Table: table,
    Players: []Player{p1, p2},
    Deck: deck,
  }
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
  return Index(c, s) == -1
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
    if c.Suite == Denari {
      a += 1
    }
  }

  var b int
  for _, c := range p2.Grabbed {
    if c.Suite == Denari {
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

  points := map[int]int {
    7: 21,
    6: 18,
    1: 16,
    5: 15,
    4: 14,
    3: 13,
    2: 12,
    10: 10,
  }

  var d,s,k,b int
  for _, c := range p.Grabbed {
    if c.Suite == Denari {
      d = max(d, points[c.Value])
    }
    if c.Suite == Spade {
      s = max(s, points[c.Value])
    }
    if c.Suite == Coppe {
      k = max(k, points[c.Value])
    }
    if c.Suite == Bastoni {
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


func (s *State) EndTurn() {
  // Move the turn to the next player.
  s.NextPlayer += 1
  if s.NextPlayer > len(s.Players) {
    s.NextPlayer = 1
  }

  // Check if this was actually the end of the game...
  if len(s.Deck) == 0 && s.EmptyHands() {

    // Give the player that last took cards, the remaining cards on the table.
    g := &s.Players[s.LastPlayerToTake - 1]
    g.Grabbed = append(g.Grabbed, s.Table...)

    // Not that it matters, but remove the last cards from the table.
    s.Table = []Card{}

    // Count points
    MostCards(&s.Players[0], &s.Players[1])
    MostDenari(&s.Players[0], &s.Players[1])
    SetteBello(&s.Players[0], &s.Players[1])
    Primera(&s.Players[0], &s.Players[1])
    return
  }

  if s.EmptyHands() {
    // Deal out the next 3 cards to each player and remove them from the deck.
    s.Players[0].Hand = s.Deck[:3]
    s.Players[1].Hand = s.Deck[3:6]
    s.Deck = s.Deck[6:]
  }

}

func (s *State) Take(card Card, table []Card) error {
  // Validating inputs...
  // Check that the math works out...
  var sum int
  for _, t := range table {
    sum += t.Value
  }
  if sum != card.Value {
    return &moveError{fmt.Sprintf("%v can't take %v", card, table)}
  }

  // Check that the cards are actually on the table.
  for _, t := range table {
    if Contains(t, s.Table) {
      return fmt.Errorf("%v is not a card on the table: %v", t, s.Table)
    }
  }

  if err := s.CheckCurrentPlayerHasCard(card); err != nil {
    return err
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

  s.EndTurn()
  return nil
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

  s.EndTurn()
  return nil
}


func main() {

  state := NewGame()
  if err := state.Check(); err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
  }

  // These are moves based on a non-random seed.
  moves := []struct{
    drop Card
    take []Card
  }{
    // Round 1, Player 1 starts
    {take: []Card{Card{Bastoni, 2}, Card{Denari, 2}}},
    {take: []Card{Card{Bastoni, 8}, Card{Coppe, 8}}},
    {take: []Card{Card{Spade, 9}, Card{Bastoni, 9}}},
    {drop: Card{Bastoni, 1}},
    {drop: Card{Bastoni, 5}},
    {drop: Card{Spade, 3}},

    // Round 2
    {take: []Card{Card{Denari, 8}, Card{Bastoni, 5}, Card{Spade, 3}}},
    {drop: Card{Coppe, 10}},
    {drop: Card{Spade, 4}},
    {take: []Card{Card{Spade, 10}, Card{Coppe, 10}}},
    {take: []Card{Card{Bastoni, 4}, Card{Spade, 4}}},
    {drop: Card{Spade, 8}},

    // Round 3
    {take: []Card{Card{Coppe, 2}, Card{Denari, 1}, Card{Bastoni, 1}}},
    {drop: Card{Denari, 5}},
    {drop: Card{Coppe, 3}},
    {take: []Card{Card{Spade, 5}, Card{Denari, 5}}},
    {drop: Card{Coppe, 7}},
    {drop: Card{Coppe, 1}},

    // Round 4
    {take: []Card{Card{Suite(1), 7}, Card{Suite(3), 7}}},
    {take: []Card{Card{Suite(1), 9}, Card{Suite(3), 1}, Card{Suite(2), 8}}},
    {drop: Card{Suite(3), 9}},
    {drop: Card{Suite(2), 1}},
    {drop: Card{Suite(2), 2}},
    {take: []Card{Card{Suite(4), 3}, Card{Suite(3), 3}}},

    // Round 5
    {take: []Card{Card{Suite(1), 10}, Card{Suite(3), 9}, Card{Suite(2), 1}}},
    {drop: Card{Suite(1), 3}},
    {drop: Card{Suite(3), 4}},
    {take: []Card{Card{Suite(1), 4}, Card{Suite(3), 4}}},
    {drop: Card{Suite(4), 6}},
    {take: []Card{Card{Suite(1), 6}, Card{Suite(4), 6}}},

    // Round 6
    {drop: Card{Suite(4), 10}},
    {take: []Card{Card{Suite(3), 5}, Card{Suite(2), 2}, Card{Suite(1), 3}}},
    {drop: Card{Suite(2), 6}},
    {take: []Card{Card{Suite(3), 6}, Card{Suite(2), 6}}},
    {drop: Card{Suite(2), 7}},
    {take: []Card{Card{Suite(4), 7}, Card{Suite(2), 7}}},
  }

  for _, m := range moves {
    if m.take != nil {
      if err := state.Take(m.take[0], m.take[1:]); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
      }
    } else {
      if err := state.Drop(m.drop); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
      }
    }
  }

  if err := state.Check(); err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
  }

  if j, err := json.MarshalIndent(state, "", "  "); err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
  } else {
    fmt.Print(string(j))
  }
}
