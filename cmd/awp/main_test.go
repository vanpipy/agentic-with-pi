package main

import (
	"reflect"
	"testing"
)

func TestExtractMaxTurnsFlagDefault(t *testing.T) {
	if n := extractMaxTurnsFlag([]string{"awp", "serve"}); n != 0 {
		t.Errorf("no flag = %d, want 0", n)
	}
}

func TestExtractMaxTurnsFlagSpace(t *testing.T) {
	if n := extractMaxTurnsFlag([]string{"awp", "serve", "--max-turns", "50"}); n != 50 {
		t.Errorf("--max-turns 50 = %d, want 50", n)
	}
}

func TestExtractMaxTurnsFlagEquals(t *testing.T) {
	if n := extractMaxTurnsFlag([]string{"awp", "serve", "--max-turns=75"}); n != 75 {
		t.Errorf("--max-turns=75 = %d, want 75", n)
	}
}

func TestExtractMaxTurnsFlagRejectsZero(t *testing.T) {
	if n := extractMaxTurnsFlag([]string{"awp", "serve", "--max-turns", "0"}); n != 0 {
		t.Errorf("--max-turns 0 should return 0 (defensive default), got %d", n)
	}
}

func TestExtractMaxTurnsFlagRejectsGarbage(t *testing.T) {
	if n := extractMaxTurnsFlag([]string{"awp", "serve", "--max-turns", "abc"}); n != 0 {
		t.Errorf("--max-turns abc should return 0, got %d", n)
	}
}

func TestStripMaxTurnsFlag(t *testing.T) {
	in := []string{"awp", "serve", "--max-turns", "50"}
	want := []string{"awp", "serve"}
	got := stripMaxTurnsFlag(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("strip = %v, want %v", got, want)
	}
}

func TestStripMaxTurnsFlagEqualsForm(t *testing.T) {
	in := []string{"awp", "serve", "--max-turns=50"}
	want := []string{"awp", "serve"}
	got := stripMaxTurnsFlag(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("strip = %v, want %v", got, want)
	}
}

func TestStripMaxTurnsFlagPreservesOthers(t *testing.T) {
	in := []string{"awp", "connect", "hello", "--max-turns", "30", "--verbose"}
	want := []string{"awp", "connect", "hello", "--verbose"}
	got := stripMaxTurnsFlag(in)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("strip = %v, want %v", got, want)
	}
}
