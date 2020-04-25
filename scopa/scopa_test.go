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
						Grabbed: []Card{Card{Suit: 1, Value: 7}, Card{Suit: 2, Value: 7}, Card{Suit: 1, Value: 10}},
						Awards:  []string{"Cards", "Denari", "SetteBello", "Primera"},
					},
					Player{Id: 2},
				},
				LastPlayerToTake: 1,
				LastMove: move{
					Take: &take{
						PlayerID: 1,
						Card:     Card{Suit: 1, Value: 7},
						Table:    []Card{{Suit: 2, Value: 7}},
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
						Grabbed: []Card{{Suit: 1, Value: 7}, {Suit: 2, Value: 7}},
						Awards:  []string{"Cards", "Denari", "SetteBello", "Primera"},
					},
					Player{Id: 2},
				},
				LastMove: move{
					Take: &take{
						PlayerID: 1,
						Card:     Card{Suit: 1, Value: 7},
						Table:    []Card{{Suit: 2, Value: 7}},
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
			card: Card{Suit: 4, Value: 9},
			take: []Card{Card{Suit: 2, Value: 9}},
			hand: []Card{Card{Suit: 4, Value: 8}, Card{Suit: 4, Value: 1}, Card{Suit: 4, Value: 9}},
			table: []Card{
				Card{Suit: 4, Value: 2},
				Card{Suit: 4, Value: 5},
				Card{Suit: 2, Value: 9},
				Card{Suit: 2, Value: 3},
			},
			wantState: &State{
				NextPlayer:       2,
				LastPlayerToTake: 1,
				Table: []Card{
					{Suit: 4, Value: 2},
					{Suit: 4, Value: 5},
					{Suit: 2, Value: 3},
				},
				Players: []Player{
					Player{
						Id:      1,
						Hand:    []Card{{Suit: 4, Value: 8}, {Suit: 4, Value: 1}},
						Grabbed: []Card{{Suit: 4, Value: 9}, {Suit: 2, Value: 9}},
					},
					Player{Id: 2},
				},
				LastMove: move{
					Take: &take{
						PlayerID: 1,
						Card:     Card{Suit: 4, Value: 9},
						Table:    []Card{{Suit: 2, Value: 9}},
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

func TestDupe(t *testing.T) {
	s := State{NextPlayer: 1, LastPlayerToTake: 0, Deck: []Card{Card{Suit: 1, Value: 8}, Card{Suit: 2, Value: 4}, Card{Suit: 4, Value: 4}, Card{Suit: 2, Value: 8}, Card{Suit: 3, Value: 10}, Card{Suit: 2, Value: 10}, Card{Suit: 3, Value: 2}, Card{Suit: 3, Value: 7}, Card{Suit: 3, Value: 3}, Card{Suit: 3, Value: 1}, Card{Suit: 1, Value: 5}, Card{Suit: 2, Value: 5}, Card{Suit: 3, Value: 9}, Card{Suit: 1, Value: 7}, Card{Suit: 2, Value: 2}, Card{Suit: 1, Value: 9}, Card{Suit: 2, Value: 1}, Card{Suit: 4, Value: 3}, Card{Suit: 3, Value: 4}, Card{Suit: 4, Value: 6}, Card{Suit: 1, Value: 10}, Card{Suit: 1, Value: 4}, Card{Suit: 1, Value: 6}, Card{Suit: 1, Value: 3}, Card{Suit: 4, Value: 10}, Card{Suit: 2, Value: 6}, Card{Suit: 2, Value: 7}, Card{Suit: 3, Value: 6}, Card{Suit: 4, Value: 7}, Card{Suit: 3, Value: 5}}, Table: []Card{Card{Suit: 4, Value: 2}, Card{Suit: 4, Value: 5}, Card{Suit: 2, Value: 9}, Card{Suit: 2, Value: 3}}, Players: []Player{Player{Id: 1, Hand: []Card{Card{Suit: 4, Value: 8}, Card{Suit: 4, Value: 1}, Card{Suit: 4, Value: 9}}, Grabbed: []Card(nil), Scopas: 0, Awards: []string(nil)}, Player{Id: 2, Hand: []Card{Card{Suit: 1, Value: 2}, Card{Suit: 1, Value: 1}, Card{Suit: 3, Value: 8}}, Grabbed: []Card(nil), Scopas: 0, Awards: []string(nil)}}, LastMove: move{Drop: (*drop)(nil), Take: (*take)(nil)}}
	if err := s.Take(Card{Suit: 4, Value: 9}, []Card{Card{Suit: 2, Value: 9}}); err != nil {
		t.Errorf("Wtf: %s\n", "foo")
	}
	w := State{NextPlayer: 1, LastPlayerToTake: 0, Deck: []Card{Card{Suit: 1, Value: 8}, Card{Suit: 2, Value: 4}, Card{Suit: 4, Value: 4}, Card{Suit: 2, Value: 8}, Card{Suit: 3, Value: 10}, Card{Suit: 2, Value: 10}, Card{Suit: 3, Value: 2}, Card{Suit: 3, Value: 7}, Card{Suit: 3, Value: 3}, Card{Suit: 3, Value: 1}, Card{Suit: 1, Value: 5}, Card{Suit: 2, Value: 5}, Card{Suit: 3, Value: 9}, Card{Suit: 1, Value: 7}, Card{Suit: 2, Value: 2}, Card{Suit: 1, Value: 9}, Card{Suit: 2, Value: 1}, Card{Suit: 4, Value: 3}, Card{Suit: 3, Value: 4}, Card{Suit: 4, Value: 6}, Card{Suit: 1, Value: 10}, Card{Suit: 1, Value: 4}, Card{Suit: 1, Value: 6}, Card{Suit: 1, Value: 3}, Card{Suit: 4, Value: 10}, Card{Suit: 2, Value: 6}, Card{Suit: 2, Value: 7}, Card{Suit: 3, Value: 6}, Card{Suit: 4, Value: 7}, Card{Suit: 3, Value: 5}}, Table: []Card{Card{Suit: 4, Value: 2}, Card{Suit: 4, Value: 5}, Card{Suit: 2, Value: 3}, Card{Suit: 2, Value: 3}}, Players: []Player{Player{Id: 1, Hand: []Card{Card{Suit: 4, Value: 8}, Card{Suit: 4, Value: 1}}, Grabbed: []Card{Card{Suit: 4, Value: 9}, Card{Suit: 2, Value: 9}}, Scopas: 0, Awards: []string(nil)}, Player{Id: 2, Hand: []Card{Card{Suit: 1, Value: 2}, Card{Suit: 1, Value: 1}, Card{Suit: 3, Value: 8}}, Grabbed: []Card(nil), Scopas: 0, Awards: []string(nil)}}, LastMove: move{Drop: (*drop)(nil), Take: (*take)(nil)}}
	if d := cmp.Diff(w, s); d != "" {
		t.Errorf("mismatch (-prod +test):\n%s", d)
	}
}
