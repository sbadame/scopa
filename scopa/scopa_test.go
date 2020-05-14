package scopa

import (
	"github.com/google/go-cmp/cmp"
	"testing"
)

func TestTake(t *testing.T) {
	var tests = map[string]struct {
		card      Card
		take      []Card
		hand      []Card
		table     []Card
		wantErr   error
		wantGame *Game
	}{
		"simple": {
			card:  Card{Denari, 7},
			take:  []Card{Card{Coppe, 7}},
			hand:  []Card{Card{Denari, 7}},
			table: []Card{Card{Coppe, 7}, Card{Denari, 10}},
			wantGame: &Game{
				NextPlayer: "2",
				Table:      []Card{},
				Players: []Player{
					Player{
						Name:    "1",
						Hand:    []Card{},
						Scopas:  0,
						Grabbed: []Card{Card{Denari, 7}, Card{Coppe, 7}},
						Awards:  []string{"Cards", "Denari", "SetteBello", "Primera"},
					},
					Player{Name: "2"},
				},
				LastPlayerToTake: "1",
				LastMove: move{
					Take: &take{
						Player: "1",
						Card:   Card{Denari, 7},
						Table:  []Card{{Coppe, 7}},
					},
				},
			},
		},
		"simple with scopa": {
			card:  Card{Denari, 7},
			take:  []Card{Card{Coppe, 7}},
			hand:  []Card{Card{Denari, 7}},
			table: []Card{Card{Coppe, 7}},
			wantGame: &Game{
				NextPlayer:       "2",
				Table:            []Card{},
				LastPlayerToTake: "1",
				Players: []Player{
					Player{
						Name:    "1",
						Hand:    []Card{},
						Scopas:  1,
						Grabbed: []Card{{Denari, 7}, {Coppe, 7}},
						Awards:  []string{"Cards", "Denari", "SetteBello", "Primera"},
					},
					Player{Name: "2"},
				},
				LastMove: move{
					Take: &take{
						Player: "1",
						Card:   Card{Denari, 7},
						Table:  []Card{{Coppe, 7}},
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
			card: Card{Spade, 9},
			take: []Card{Card{Coppe, 9}},
			hand: []Card{Card{Spade, 8}, Card{Spade, 1}, Card{Spade, 9}},
			table: []Card{
				Card{Spade, 2},
				Card{Spade, 5},
				Card{Coppe, 9},
				Card{Coppe, 3},
			},
			wantGame: &Game{
				NextPlayer:       "2",
				LastPlayerToTake: "1",
				Table: []Card{
					{Spade, 2},
					{Spade, 5},
					{Coppe, 3},
				},
				Players: []Player{
					Player{
						Name:    "1",
						Hand:    []Card{{Spade, 8}, {Spade, 1}},
						Grabbed: []Card{{Spade, 9}, {Coppe, 9}},
					},
					Player{Name: "2"},
				},
				LastMove: move{
					Take: &take{
						Player: "1",
						Card:   Card{Spade, 9},
						Table:  []Card{{Coppe, 9}},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		s := Game{
			NextPlayer: "1",
			Table:      tc.table,
			Players: []Player{
				Player{Name: "1", Hand: tc.hand},
				Player{Name: "2"},
			},
		}

		err := s.Take(tc.card, tc.take)
		if d := cmp.Diff(err, tc.wantErr); d != "" {
			t.Errorf("%s: mismatch error (-want +got):\n%s", name, d)
		}

		if tc.wantGame != nil {
			if d := cmp.Diff(*tc.wantGame, s); d != "" {
				t.Errorf("%s: mismatch state (-want +got):\n%s", name, d)
			}
		}
	}
}
