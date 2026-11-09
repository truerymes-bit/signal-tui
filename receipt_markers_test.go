package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func receiptMarkerLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		s := ansi.Strip(line)
		if strings.Contains(s, "You:") {
			continue
		}
		if strings.Contains(s, "│") && (strings.Contains(s, "*") || strings.Contains(s, "✓") || strings.Contains(s, "v")) {
			return line
		}
	}
	return ""
}

func TestReceiptMarkersDistinct(t *testing.T) {
	for _, name := range namedThemeKeys() {
		th := ThemeForName(name)
		var lines []string
		for _, r := range []int{0, 1, 2} {
			m := newModel()
			m.state = stateChat
			m.width = 80
			m.height = 24
			m.theme = th
			m.themeName = name
			sent := true
			m.messages = []Message{{IsMe: true, Text: "hi", Sent: sent, Receipt: r}}
			out := m.viewChat()
			rl := receiptMarkerLine(out)
			if rl == "" {
				t.Fatalf("%s receipt %d: no receipt marker line found", name, r)
			}
			lines = append(lines, rl)
		}
		// pairwise distinct (full rendered line includes color codes)
		for i := 0; i < len(lines); i++ {
			for j := i + 1; j < len(lines); j++ {
				if lines[i] == lines[j] {
					t.Errorf("%s: receipt markers %d and %d are identical: %q", name, i, j, ansi.Strip(lines[i]))
				}
			}
		}
	}
}
