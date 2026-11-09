package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
)

func TestReceivedFirstLinePadding(t *testing.T) {
	for _, name := range themeNamesPlusDefault() {
		var th Theme
		if name == "default" {
			th = NewTheme(colorprofile.Detect(nil, nil), true)
		} else {
			th = ThemeForName(name)
		}
		m := newModel()
		if name != "default" {
			m.themeName = name
		}
		m.theme = th
		m.state = stateChat
		m.width = 100
		m.height = 30
		m.contacts = []Contact{{Number: "contact", Name: "contact"}}
		m.selected = 0
		now := time.Now()
		longText := strings.Repeat("word ", 40)
		m.messages = []Message{{From: "contact", Text: longText, Time: now, IsMe: false, Contact: "contact"}}
		m.history = map[string][]Message{"contact": m.messages}

		v := m.View()
		rows, cells := parseScreenCells(v.Content)

		start := -1
		for i, r := range rows {
			runes := []rune(r)
			if strings.Contains(string(runes), "contact:") {
				// ensure it's in the chat pane (after col 20)
				start = i
				break
			}
		}
		if start < 0 {
			t.Fatalf("theme %q: message row not found", name)
		}

		runes := []rune(rows[start])
		prefixRunes := []rune("contact: ")
		prefixIdx := -1
		for i := 20; i+len(prefixRunes) <= len(runes); i++ {
			if string(runes[i:i+len(prefixRunes)]) == "contact: " {
				prefixIdx = i
				break
			}
		}
		if prefixIdx < 0 {
			t.Fatalf("theme %q: prefix not found", name)
		}
		bodyCol := prefixIdx + len(prefixRunes)

		leftEdge := -1
		for col := 0; col < len(runes); col++ {
			if cells[[2]int{start, col}] == inBgToken(th) {
				leftEdge = col
				break
			}
		}
		if leftEdge < 0 {
			t.Fatalf("theme %q: no InBg cell found", name)
		}
		if prefixIdx != leftEdge+1 {
			t.Fatalf("theme %q: row %d prefix col %d != leftEdge+1 %d", name, start, prefixIdx, leftEdge+1)
		}
		if leftEdge-1 >= 0 && cells[[2]int{start, leftEdge - 1}] == inBgToken(th) {
			t.Fatalf("theme %q: row %d has more than one leading padding space", name, start)
		}

		row2 := start + 1
		if row2 >= len(rows) {
			t.Fatalf("theme %q: missing continuation row", name)
		}
		row2Runes := []rune(rows[row2])
		row2Col := -1
		for col := leftEdge; col < leftEdge+71 && col < len(row2Runes); col++ {
			if row2Runes[col] != ' ' {
				row2Col = col
				break
			}
		}
		if row2Col != bodyCol {
			t.Fatalf("theme %q: row %d body col %d != row %d first glyph col %d", name, start, bodyCol, row2, row2Col)
		}
	}
}
