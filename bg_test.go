package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

func colorHex(c color.Color) string {
	if rgba, ok := c.(color.RGBA); ok {
		return fmt.Sprintf("#%02x%02x%02x", rgba.R, rgba.G, rgba.B)
	}
	return ""
}

func hexRGB(hex string) (int, int, int) {
	h := strings.TrimPrefix(hex, "#")
	r, _ := strconv.ParseInt(h[0:2], 16, 0)
	g, _ := strconv.ParseInt(h[2:4], 16, 0)
	b, _ := strconv.ParseInt(h[4:6], 16, 0)
	return int(r), int(g), int(b)
}

func colorEscape(hex string) string {
	r, g, b := hexRGB(hex)
	return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
}

func TestBgEveryTheme(t *testing.T) {
	for _, name := range namedThemeKeys() {
		th := ThemeForName(name)
		if th.P.Bg == nil {
			t.Errorf("theme %q: P.Bg is nil", name)
			continue
		}
		if th.P.Bg == th.P.InBg {
			t.Errorf("theme %q: P.Bg equals P.InBg", name)
		}
		m := newModel()
		m.themeName = name
		m.theme = th
		m.state = stateChat
		m.width = 80
		m.height = 24
		m.contacts = []Contact{{Number: "contact", Name: "contact"}}
		m.selected = 0
		m.history = map[string][]Message{"contact": {{From: "contact", Text: "hello", Time: time.Now(), IsMe: false, Contact: "contact"}}}
		v := m.View()
		bgEsc := colorEscape(colorHex(th.P.Bg))
		inBgEsc := colorEscape(colorHex(th.P.InBg))
		if !strings.Contains(v.Content, bgEsc) {
			t.Errorf("theme %q: pane Bg escape %s not found", name, bgEsc)
		}
		if !strings.Contains(v.Content, inBgEsc) {
			t.Errorf("theme %q: received InBg escape %s not found", name, inBgEsc)
		}
		if bgEsc == inBgEsc {
			t.Errorf("theme %q: Bg and InBg escapes identical", name)
		}
		if lipgloss.Height(v.Content) != 24 {
			t.Errorf("theme %q: height %d != 24", name, lipgloss.Height(v.Content))
		}
	}
	fmt.Println("bg check done")
}
