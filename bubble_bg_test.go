package main

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func rgbaHex(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func applySGR(params ansi.Params, curBg *string) {
	i := 0
	for i < len(params) {
		pv, _, _ := params.Param(i, 0)
		switch {
		case pv == 0:
			*curBg = "default"
		case pv == 49:
			*curBg = "default"
		case pv >= 40 && pv <= 47:
			// not needed for themes, but keep
		case pv == 48:
			mode, _, _ := params.Param(i+1, 0)
			if mode == 5 {
				n, _, _ := params.Param(i+2, 0)
				*curBg = "48hex#" + xterm256(n)
				i += 2
			} else if mode == 2 {
				r, _, _ := params.Param(i+2, 0)
				g, _, _ := params.Param(i+3, 0)
				b, _, _ := params.Param(i+4, 0)
				*curBg = fmt.Sprintf("48hex#%02x%02x%02x", r, g, b)
				i += 4
			}
		}
		i++
	}
}

func xterm256(n int) string {
	if n < 16 {
		basic := []string{
			"000000", "800000", "008000", "808000", "000080", "800080", "008080", "c0c0c0",
			"808080", "ff0000", "00ff00", "ffff00", "0000ff", "ff00ff", "00ffff", "ffffff",
		}
		if n < len(basic) {
			return basic[n]
		}
		return ""
	}
	if n >= 232 {
		v := 8 + (n-232)*10
		return fmt.Sprintf("%02x%02x%02x", v, v, v)
	}
	n -= 16
	r := n / 36
	g := (n / 6) % 6
	b := n % 6
	toVal := func(x int) int {
		if x == 0 {
			return 0
		}
		return 55 + x*40
	}
	return fmt.Sprintf("%02x%02x%02x", toVal(r), toVal(g), toVal(b))
}
func themeNamesPlusDefault() []string {
	names := append([]string{}, namedThemeKeys()...)
	names = append(names, "default")
	return names
}

func inBgToken(th Theme) string {
	hex := rgbaHex(th.P.InBg)
	if hex == "" {
		hex = colorHex(th.P.InBg)
	}
	if hex == "" {
		return ""
	}
	return "48hex#" + strings.TrimPrefix(hex, "#")
}

func parseScreenCells(content string) ([]string, map[[2]int]string) {
	// returns row text slice and cell bg map
	cellBg := map[[2]int]string{}
	rowsTextMap := map[int]string{}
	curBg := "default"
	row, col := 0, 0
	p := ansi.NewParser()
	p.SetHandler(ansi.Handler{
		Print: func(r rune) {
			cellBg[[2]int{row, col}] = curBg
			rowsTextMap[row] += string(r)
			col++
		},
		Execute: func(b byte) {
			if b == '\n' {
				row++
				col = 0
			}
		},
		HandleCsi: func(cmd ansi.Cmd, params ansi.Params) {
			if cmd.Final() == 'm' {
				applySGR(params, &curBg)
			}
		},
	})
	p.Parse([]byte(content))
	maxRow := 0
	for r := range rowsTextMap {
		if r > maxRow {
			maxRow = r
		}
	}
	rows := make([]string, maxRow+1)
	for r, t := range rowsTextMap {
		rows[r] = t
	}
	return rows, cellBg
}

func TestReceivedBubbleBgFullWidth(t *testing.T) {
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
		} else {
			m.themeName = ""
		}
		m.theme = th
		m.state = stateChat
		m.width = 100
		m.height = 30
		m.contacts = []Contact{{Number: "contact", Name: "contact"}}
		m.selected = 0
		now := time.Now()
		longText := "this is a long received message that should wrap to two lines for testing background"
		m.messages = []Message{{From: "contact", Text: longText, Time: now, IsMe: false, Contact: "contact"}}
		m.history = map[string][]Message{"contact": m.messages}

		v := m.View()
		rows, cells := parseScreenCells(v.Content)

		start := -1
		for i, r := range rows {
			if strings.Contains(r, "contact:") {
				start = i
				break
			}
		}
		if start < 0 {
			t.Fatalf("theme %q: message row not found", name)
		}
		want := inBgToken(th)
		// bubble columns 27..97 inclusive
		for r := start; r < start+2; r++ {
			if r >= len(rows) {
				t.Fatalf("theme %q: missing row %d", name, r)
			}
			for col := 27; col <= 97; col++ {
				if got := cells[[2]int{r, col}]; got != want {
					t.Fatalf("theme %q: row %d col %d bg=%s want=%s", name, r, col, got, want)
				}
			}
		}
	}
}
