package scopa

import (
	"testing"
)

func TestDoubleCard(t *testing.T) {
	s := State{NextPlayer:1, LastPlayerToTake:1, Deck:[]Card{Card{Suite:2, Value:2}, Card{Suite:2, Value:7}, Card{Suite:4, Value:10}, Card{Suite:3, Value:6}, Card{Suite:1, Value:2}, Card{Suite:2, Value:8}, Card{Suite:4, Value:7}, Card{Suite:1, Value:5}, Card{Suite:3, Value:3}, Card{Suite:3, Value:7}, Card{Suite:3, Value:4}, Card{Suite:4, Value:6}, Card{Suite:1, Value:1}, Card{Suite:4, Value:4}, Card{Suite:4, Value:5}, Card{Suite:1, Value:6}, Card{Suite:1, Value:3}, Card{Suite:3, Value:5}, Card{Suite:4, Value:1}, Card{Suite:2, Value:10}, Card{Suite:2, Value:3}, Card{Suite:1, Value:10}, Card{Suite:2, Value:9}, Card{Suite:4, Value:2}, Card{Suite:2, Value:4}, Card{Suite:3, Value:2}, Card{Suite:4, Value:9}, Card{Suite:3, Value:10}, Card{Suite:1, Value:7}, Card{Suite:3, Value:9}}, Table:[]Card{Card{Suite:2, Value:5}, Card{Suite:2, Value:6}, Card{Suite:3, Value:8}, Card{Suite:4, Value:3}}, Players:[]Player{Player{Id:1, Hand:[]Card{Card{Suite:3, Value:1}, Card{Suite:2, Value:1}}, Grabbed:[]Card{Card{Suite:4, Value:8}, Card{Suite:1, Value:8}}, Scopas:0, Awards:[]string(nil)}, Player{Id:2, Hand:[]Card{Card{Suite:1, Value:4}, Card{Suite:1, Value:9}}, Grabbed:[]Card(nil), Scopas:0, Awards:[]string(nil)}}}

	if err := s.Drop(Card{Suite:2, Value:1}); err != nil {
		t.Errorf("Found duplicated card.")
	}
}
