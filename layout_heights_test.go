package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestLayoutHeights(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}, {60, 20}} {
		m := newModel()
		m.state = stateChat
		m.width = sz.w
		m.height = sz.h
		content := m.viewChat()
		l := computeLayout(sz.w, sz.h)
		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			lines = lines[:len(lines)-1]
		} // drop status bar
		lastSidebar, lastChat := -1, -1
		for i, ln := range lines {
			if len(ln) < l.SidebarWidth {
				ln = ln + strings.Repeat(" ", l.SidebarWidth-len(ln))
			}
			sidebarPart := ln[:l.SidebarWidth]
			chatPart := ""
			if len(ln) > l.SidebarWidth {
				chatPart = ln[l.SidebarWidth:]
			}
			if strings.TrimSpace(sidebarPart) != "" {
				lastSidebar = i
			}
			if strings.TrimSpace(chatPart) != "" {
				lastChat = i
			}
		}
		if lastSidebar != lastChat {
			t.Fatalf("%dx%d: sidebar bottom row %d != chat bottom row %d", sz.w, sz.h, lastSidebar, lastChat)
		}
		if lipgloss.Height(content) != sz.h-1 {
			// top row only; status bar added by View
		}
	}
	// also check full View height
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}, {60, 20}} {
		m := newModel()
		m.state = stateChat
		m.width = sz.w
		m.height = sz.h
		v := m.View()
		if lipgloss.Height(v.Content) != sz.h {
			t.Fatalf("View %dx%d: height %d != %d", sz.w, sz.h, lipgloss.Height(v.Content), sz.h)
		}
	}
}
