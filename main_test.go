package main

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTruncateToWidth(t *testing.T) {
	styled := "\x1b[31mhello world\x1b[0m"
	for _, w := range []int{0, 1, 3, 5, 11, 100} {
		got := truncateToWidth(styled, w)
		if w <= 0 {
			if got != "" {
				t.Fatalf("width %d: expected empty, got %q", w, got)
			}
			continue
		}
		if ansi.StringWidth(got) > w {
			t.Fatalf("width %d: result width %d exceeds limit: %q", w, ansi.StringWidth(got), got)
		}
	}
	short := "hi"
	if got := truncateToWidth(short, 10); got != short {
		t.Fatalf("short string changed: got %q want %q", got, short)
	}
}
