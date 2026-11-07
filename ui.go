package main

import (
	"hash/fnv"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

type Mode int

const (
	ModeTTY Mode = iota
	ModeRich
)

func ModeFor(p colorprofile.Profile) Mode {
	if os.Getenv("TERM") == "linux" {
		return ModeTTY
	}
	switch p {
	case colorprofile.ANSI, colorprofile.ASCII, colorprofile.NoTTY:
		return ModeTTY
	}
	return ModeRich
}

type Palette struct {
	Accent, AccentBg, Text, Muted, Border, SelBg, InBg color.Color
	OnAccent, Green, Red, Yellow                       color.Color
	Sender                                             []color.Color
}

func NewPalette(p colorprofile.Profile, isDark bool) Palette {
	complete := lipgloss.Complete(p)
	ld := lipgloss.LightDark(isDark)
	c := func(hex, a256, a16 string) color.Color {
		return complete(lipgloss.Color(a16), lipgloss.Color(a256), lipgloss.Color(hex))
	}
	return Palette{
		Accent:   c("#3A76F0", "69", "12"),
		AccentBg: c("#3A76F0", "62", "4"),
		Text:     ld(lipgloss.Color("#1A1A2E"), lipgloss.Color("#C0CAF5")),
		Muted:    ld(lipgloss.Color("#565F89"), lipgloss.Color("#565F89")),
		Border:   ld(lipgloss.Color("#A9B1D6"), lipgloss.Color("#3B4261")),
		SelBg:    ld(lipgloss.Color("#D9D9D9"), lipgloss.Color("#283457")),
		InBg:     ld(lipgloss.Color("#E8E8E8"), lipgloss.Color("#292E42")),
		OnAccent: c("#FFFFFF", "231", "15"),
		Green:    c("#9ECE6A", "114", "2"),
		Red:      c("#F7768E", "204", "9"),
		Yellow:   c("#E0AF68", "179", "3"),
		Sender: []color.Color{
			c("#BB9AF7", "141", "5"), c("#7DCFFF", "117", "6"),
			c("#E0AF68", "179", "3"), c("#9ECE6A", "114", "2"),
			c("#F7768E", "204", "1"), c("#73DACA", "79", "14"),
		},
	}
}

func (p Palette) SenderColor(name string) color.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return p.Sender[int(h.Sum32())%len(p.Sender)]
}

type Glyphs struct {
	Unread, Online, Sent, Delivered, Read, Bar, Prompt, Sep string
}

type Theme struct {
	Mode   Mode
	P      Palette
	G      Glyphs
	Border lipgloss.Border
	IsDark bool

	PaneFocused, PaneBlurred lipgloss.Style
	Title                    lipgloss.Style
	ChatItem, ChatItemSel    lipgloss.Style
	Unread                   lipgloss.Style
	MsgIn, MsgOut            lipgloss.Style
	Time, Meta               lipgloss.Style
	Input                    lipgloss.Style
	StatusBar, StatusKey     lipgloss.Style
	Error                    lipgloss.Style
}

func NewTheme(profile colorprofile.Profile, isDark bool) Theme {
	m := ModeFor(profile)
	p := NewPalette(profile, isDark)
	t := Theme{Mode: m, P: p, IsDark: isDark}

	if m == ModeRich {
		t.Border = lipgloss.RoundedBorder()
		t.G = Glyphs{"●", "●", "✓", "✓✓", "✓✓", "┃", "❯", "·"}
	} else {
		t.Border = lipgloss.NormalBorder()
		t.G = Glyphs{"*", "o", "v", "vv", "vv", "|", ">", "-"}
	}

	base := lipgloss.NewStyle().Border(t.Border)
	t.PaneBlurred = base.BorderForeground(p.Border)
	t.PaneFocused = base.BorderForeground(p.Accent)
	t.Title = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)
	t.Time = lipgloss.NewStyle().Foreground(p.Muted)
	t.Meta = lipgloss.NewStyle().Foreground(p.Muted).Italic(m == ModeRich)
	t.Unread = lipgloss.NewStyle().Foreground(p.Red).Bold(true)
	t.Error = lipgloss.NewStyle().Foreground(p.Red)
	t.Input = lipgloss.NewStyle().Foreground(p.Text)
	t.StatusKey = lipgloss.NewStyle().Foreground(p.Accent).Bold(true)

	if m == ModeRich {
		t.ChatItem = lipgloss.NewStyle().Foreground(p.Text).PaddingLeft(1)
		t.ChatItemSel = lipgloss.NewStyle().Foreground(p.OnAccent).Background(p.SelBg).Bold(true).PaddingLeft(1)
		t.MsgIn = lipgloss.NewStyle().Foreground(p.Text).Background(p.InBg).Padding(0, 1)
		t.MsgOut = lipgloss.NewStyle().Foreground(p.OnAccent).Background(p.AccentBg).Padding(0, 1)
		t.StatusBar = lipgloss.NewStyle().Foreground(p.Muted).Background(p.InBg).Padding(0, 1)
	} else {
		t.ChatItem = lipgloss.NewStyle().Foreground(p.Text).PaddingLeft(1)
		t.ChatItemSel = lipgloss.NewStyle().Reverse(true).Bold(true).PaddingLeft(1)
		t.MsgIn = lipgloss.NewStyle().Foreground(p.Text).PaddingLeft(1)
		t.MsgOut = lipgloss.NewStyle().Foreground(p.Accent).PaddingRight(1)
		t.StatusBar = lipgloss.NewStyle().Reverse(true).Padding(0, 1)
	}
	return t
}

func (t Theme) Status(w int, left, right string) string {
	gap := w - 2 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return t.StatusBar.Width(w).Render(left + strings.Repeat(" ", gap) + right)
}

func (t Theme) Hint(key, label string) string {
	return t.StatusKey.Render(key) + t.Time.Render(" "+label)
}
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
