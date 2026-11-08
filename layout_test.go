package main

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestNoOverflow(t *testing.T) {
	for w := 20; w <= 220; w++ {
		for _, h := range []int{8, 12, 24, 40} {
			m := newModel()
			m.state = stateChat
			m.width = w
			m.height = h
			content := m.viewChat()
			cw, ch := lipgloss.Size(content)
			if cw > w || ch > h {
				t.Fatalf("overflow at %dx%d: got %dx%d", w, h, cw, ch)
			}
		}
	}
}
