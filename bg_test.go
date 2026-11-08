package main

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func hasBg(s string) bool {
	// simple: check if output contains a background SGR sequence
	return strings.Contains(s, "48;")
}

func TestBgEveryTheme(t *testing.T) {
	themes := []string{"", "solarized-light", "dracula", "nord"}
	_ = ansi.StringWidth
	for _, name := range themes {
		for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
			m := newModel()
			if name != "" {
				m.theme = ThemeForName(name)
			}
			m.state = stateChat
			m.width = sz.w
			m.height = sz.h
			v := m.View()
			if !hasBg(v.Content) {
				t.Fatalf("theme %q %dx%d: no background SGR found", name, sz.w, sz.h)
			}
			if lipgloss.Height(v.Content) != sz.h {
				t.Fatalf("theme %q %dx%d: height %d != %d", name, sz.w, sz.h, lipgloss.Height(v.Content), sz.h)
			}
		}
	}
	fmt.Println("bg check done")
}
