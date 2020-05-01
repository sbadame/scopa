package scopa

import (
	"github.com/google/go-cmp/cmp"
	"reflect"
	"testing"
)

func TestTake(t *testing.T) {
	var tests = map[string]struct {
		card      Card
		take      []Card
		hand      []Card
		table     []Card
		wantErr   error
		wantState *State
	}{
		"simple": {
			card:  Card{Denari, 7},
			take:  []Card{Card{Coppe, 7}},
			hand:  []Card{Card{Denari, 7}},
			table: []Card{Card{Coppe, 7}, Card{Denari, 10}},
			wantState: &State{
				NextPlayer: 2,
				Table:      []Card{},
				Players: []Player{
					Player{
						Id:      1,
						Hand:    []Card{},
						Scopas:  0,
						Grabbed: []Card{Card{Suit: Denari, Value: 7}, Card{Suit: Coppe, Value: 7}, Card{Suit: Denari, Value: 10}},
						Awards:  []string{"Cards", "Denari", "SetteBello", "Primera"},
					},
					Player{Id: 2},
				},
				LastPlayerToTake: 1,
				LastMove: move{
					Take: &take{
						PlayerID: 1,
						Card:     Card{Suit: Denari, Value: 7},
						Table:    []Card{{Suit: Coppe, Value: 7}},
					},
				},
			},
		},
		"simple with scopa": {
			card:  Card{Denari, 7},
			take:  []Card{Card{Coppe, 7}},
			hand:  []Card{Card{Denari, 7}},
			table: []Card{Card{Coppe, 7}},
			wantState: &State{
				NextPlayer:       2,
				Table:            []Card{},
				LastPlayerToTake: 1,
				Players: []Player{
					Player{
						Id:      1,
						Hand:    []Card{},
						Scopas:  1,
						Grabbed: []Card{{Suit: Denari, Value: 7}, {Suit: Coppe, Value: 7}},
						Awards:  []string{"Cards", "Denari", "SetteBello", "Primera"},
					},
					Player{Id: 2},
				},
				LastMove: move{
					Take: &take{
						PlayerID: 1,
						Card:     Card{Suit: Denari, Value: 7},
						Table:    []Card{{Suit: Coppe, Value: 7}},
					},
				},
			},
		},
		"doesn't add up": {
			card:    Card{Denari, 7},
			take:    []Card{Card{Coppe, 8}},
			hand:    []Card{Card{Denari, 7}},
			table:   []Card{Card{Coppe, 8}},
			wantErr: badMathTake(Card{Denari, 7}, []Card{Card{Coppe, 8}}),
		},
		"not a card on the table": {
			card:    Card{Denari, 7},
			take:    []Card{Card{Coppe, 7}},
			hand:    []Card{Card{Denari, 7}},
			table:   []Card{Card{Coppe, 8}},
			wantErr: cardMissingFromTable(Card{Coppe, 7}, []Card{Card{Coppe, 8}}),
		},
		"must take the face": {
			card:    Card{Denari, 10},
			take:    []Card{Card{Coppe, 7}, Card{Coppe, 3}},
			hand:    []Card{Card{Denari, 10}},
			table:   []Card{Card{Coppe, 7}, Card{Coppe, 3}, Card{Spade, 10}},
			wantErr: perroError(Card{Spade, 10}),
		},
		"regression": {
			card: Card{Suit: Spade, Value: 9},
			take: []Card{Card{Suit: Coppe, Value: 9}},
			hand: []Card{Card{Suit: Spade, Value: 8}, Card{Suit: Spade, Value: 1}, Card{Suit: Spade, Value: 9}},
			table: []Card{
				Card{Suit: Spade, Value: 2},
				Card{Suit: Spade, Value: 5},
				Card{Suit: Coppe, Value: 9},
				Card{Suit: Coppe, Value: 3},
			},
			wantState: &State{
				NextPlayer:       2,
				LastPlayerToTake: 1,
				Table: []Card{
					{Suit: Spade, Value: 2},
					{Suit: Spade, Value: 5},
					{Suit: Coppe, Value: 3},
				},
				Players: []Player{
					Player{
						Id:      1,
						Hand:    []Card{{Suit: Spade, Value: 8}, {Suit: Spade, Value: 1}},
						Grabbed: []Card{{Suit: Spade, Value: 9}, {Suit: Coppe, Value: 9}},
					},
					Player{Id: 2},
				},
				LastMove: move{
					Take: &take{
						PlayerID: 1,
						Card:     Card{Suit: Spade, Value: 9},
						Table:    []Card{{Suit: Coppe, Value: 9}},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		s := State{NextPlayer: 1, Table: tc.table, Players: []Player{Player{Id: 1, Hand: tc.hand}, Player{Id: 2}}}
		initialState := s
		if err := s.Take(tc.card, tc.take); !reflect.DeepEqual(err, tc.wantErr) {
			t.Errorf("%s: wanted %#v but got %#v", name, tc.wantErr, err)
			if err != nil && !reflect.DeepEqual(initialState, s) {
				t.Errorf("%s: An error was generated, but state was modified from %#v to %#v", name, initialState, s)
			}
		}
		if tc.wantState != nil {
			if d := cmp.Diff(*tc.wantState, s); d != "" {
				t.Errorf("%s: mismatch (-want +got):\n%s", name, d)
			}
		}
	}
}
