package main

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

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
	}
	m := newModel()
	m.themeName = "moss"
	m.theme = ThemeForName("moss")
	m.state = stateChat
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		m.width = sz.w
		m.height = sz.h
		v := m.View()
		if !strings.Contains(v.Content, "48;2;30;36;33") {
			t.Fatalf("moss %dx%d: Bg escape not found", sz.w, sz.h)
		}
		if lipgloss.Height(v.Content) != sz.h {
			t.Fatalf("moss %dx%d: height %d != %d", sz.w, sz.h, lipgloss.Height(v.Content), sz.h)
		}
	}
	fmt.Println("bg check done")
}
