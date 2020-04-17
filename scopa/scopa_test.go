package scopa

import (
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
		wantState State
	}{
		"simple": {
			card:      Card{Denari, 7},
			take:      []Card{Card{Coppe, 7}},
			hand:      []Card{Card{Denari, 7}},
			table:     []Card{Card{Coppe, 7}, Card{Denari, 10}},
			wantState: State{NextPlayer: 2, Table: []Card{}, Players: []Player{Player{Id: 1, Hand: []Card{}, Scopas: 1}, Player{Id: 2}}},
		},
		"simple with scopa": {
			card:      Card{Denari, 7},
			take:      []Card{Card{Coppe, 7}},
			hand:      []Card{Card{Denari, 7}},
			table:     []Card{Card{Coppe, 7}},
			wantState: State{NextPlayer: 2, Table: []Card{}, Players: []Player{Player{Id: 1, Hand: []Card{}, Scopas: 1}, Player{Id: 2}}},
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
		if reflect.DeepEqual(s, tc.wantState) {
			t.Errorf("%s: wanted %#v but got %#v", name, tc.wantState, s)
		}
	}
}
