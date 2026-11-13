package tui

import "testing"

func TestMarquee(t *testing.T) {
	if got := marquee("short", 10, 3); got != "short" {
		t.Errorf("short text scrolled: %q", got)
	}
	if got := marquee("abcdefghij", 4, 0); got != "abcd" {
		t.Errorf("start = %q", got)
	}
	if got := marquee("abcdefghij", 4, 8); got != "ij  " {
		t.Errorf("wrap = %q", got)
	}
}

func TestFlattenCommandShowsNewlines(t *testing.T) {
	if got := flattenCommand("ls\nrm -rf x"); got != "ls ⏎ rm -rf x" {
		t.Errorf("flatten = %q", got)
	}
}
