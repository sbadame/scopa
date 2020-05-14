package main

import (
	"github.com/google/go-cmp/cmp"
	"io/ioutil"
	"testing"
)

func TestMatch(t *testing.T) {
	m := Match{}
	sb := make(scoreboard)
	if _, err := m.addPlayer(-1, "a", sb); err != nil {
		t.Errorf("Couldn't join a match: %v", err)
	}

	if _, err := m.addPlayer(-1, "b", sb); err != nil {
		t.Errorf("Couldn't join a match: %v", err)
	}

}

func TestScoreboard(t *testing.T) {
	f, err := ioutil.TempFile("", "testscoreboard")
	if err != nil {
		t.Errorf("Couldn't create a tempfile.")
	}

	sb := make(scoreboard)

	sb.record("a", "b", 2, 5)
	if np := sb.nextPlayer("a", "b"); np != "b" {
		t.Errorf("1 Expected the nextPlayer to be 'b' but was '%s'.", np)
	}

	sb.record("a", "b", 2, 5)
	if np := sb.nextPlayer("a", "b"); np != "a" {
		t.Errorf("2 Expected the nextPlayer to be 'a' but was '%s'.", np)
	}

	sb.save(f.Name())

	loaded := loadScoreboard(f.Name())

	if d := cmp.Diff(sb.scores("a", "b"), map[string]int{"a": 4, "b": 10}); d != "" {
		t.Errorf("mismatch (-got, +wanted):\n%s", d)
	}

	if d := cmp.Diff(sb, loaded); d != "" {
		t.Errorf("mismatch (-saved, +loaded):\n%s", d)
	}

}
