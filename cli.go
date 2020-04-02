package main

// TODO:
//  1. Re-shuffling if the table has 3-4 Kings on the initial deal.
//  2. Forcing a place to make a trick if they can.
//  3. Forcing that only captures a single card and not multiple cards.
//  4. Support for 4 players?

import (
	"encoding/json"
	"fmt"
	"os"
	. "github.com/sbadame/scopa/scopa"
)

func main() {

	state := NewGame()
	if err := state.Check(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// These are moves based on a non-random seed.
	moves := []struct {
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
