package main

import (
	"hash/fnv"
	"image/color"
	"os"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/filepicker"
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
	return buildTheme(p, m, defaultGlyphs(m))
}

func defaultGlyphs(m Mode) Glyphs {
	if m == ModeRich {
		return Glyphs{"●", "●", "✓", "✓✓", "✓✓", "┃", "❯", "·"}
	}
	return Glyphs{"*", "o", "v", "vv", "vv", "|", ">", "-"}
}

// buildTheme constructs the complete Theme for a resolved palette and mode.
func buildTheme(p Palette, m Mode, g Glyphs) Theme {
	t := Theme{Mode: m, P: p, G: g}
	if m == ModeRich {
		t.Border = lipgloss.RoundedBorder()
	} else {
		t.Border = lipgloss.NormalBorder()
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
		t.ChatItemSel = lipgloss.NewStyle().Foreground(p.Text).Background(p.SelBg).Bold(true).PaddingLeft(1)
		t.MsgIn = lipgloss.NewStyle().Foreground(p.Text).Background(p.InBg).Padding(0, 1)
		t.MsgOut = lipgloss.NewStyle().Foreground(p.OnAccent).Background(p.AccentBg).Padding(0, 1)
		t.StatusBar = lipgloss.NewStyle().Foreground(p.Muted).Background(p.InBg).Padding(0, 1)
		t.StatusKey = lipgloss.NewStyle().Foreground(p.Accent).Background(p.InBg).Bold(true)
		t.Time = lipgloss.NewStyle().Foreground(p.Muted).Background(p.InBg)
	} else {
		t.ChatItem = lipgloss.NewStyle().Foreground(p.Text).PaddingLeft(1)
		t.ChatItemSel = lipgloss.NewStyle().Reverse(true).Bold(true).PaddingLeft(1)
		t.MsgIn = lipgloss.NewStyle().Foreground(p.Text).PaddingLeft(1)
		t.MsgOut = lipgloss.NewStyle().Foreground(p.Accent).PaddingRight(1)
		t.StatusBar = lipgloss.NewStyle().Reverse(true).Padding(0, 1)
	}
	return t
}

var spaceRunRE = regexp.MustCompile(` {2,}`)

func (t Theme) Status(w int, left, right string) string {
	gap := w - 2 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	// Style runs of spaces inside `right` (e.g. the gap between hints) so
	// they inherit the bar background instead of resetting to the terminal's.
	spaceStyle := t.StatusBar.Padding(0, 0)
	styledRight := spaceRunRE.ReplaceAllStringFunc(right, func(s string) string {
		return spaceStyle.Render(s)
	})
	return t.StatusBar.Width(w).Render(left + strings.Repeat(" ", gap) + styledRight)
}

func (t Theme) Hint(key, label string) string {
	return t.StatusKey.Render(key) + t.Time.Render(" "+label)
}

// FilePickerStyles maps the theme onto the bubbles filepicker's public
// Styles fields so the picker follows the same palette and mode as the app.
func (t Theme) FilePickerStyles() filepicker.Styles {
	var st filepicker.Styles
	style := func(fg color.Color) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(fg)
	}
	st.Directory = style(t.P.Text)
	st.File = style(t.P.Text)
	st.Permission = style(t.P.Muted)
	st.FileSize = style(t.P.Muted)
	st.Symlink = style(t.P.Text)
	st.EmptyDirectory = style(t.P.Muted)
	if t.Mode == ModeRich {
		sel := lipgloss.NewStyle().Foreground(t.P.OnAccent).Background(t.P.SelBg).Bold(true)
		st.Cursor = sel
		st.Selected = sel
		st.DisabledSelected = lipgloss.NewStyle().Foreground(t.P.Muted).Background(t.P.SelBg).Bold(true)
		st.DisabledCursor = st.DisabledSelected
		st.DisabledFile = style(t.P.Muted)
	} else {
		sel := lipgloss.NewStyle().Reverse(true).Bold(true)
		st.Cursor = sel
		st.Selected = sel
		st.DisabledSelected = sel
		st.DisabledCursor = sel
		st.DisabledFile = style(t.P.Muted)
	}
	return st
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

// ThemeConfig is one complete named theme: palette, glyphs, and appearance.
type ThemeConfig struct {
	Name string
	Palette
	Glyphs
	IsDark bool
}

// NamedThemes lists the themes the engine can resolve.
var NamedThemes = map[string]ThemeConfig{
	"cyberpunk": {
		Name: "Cyberpunk",
		Palette: Palette{
			Accent:   lipgloss.Color("#FF007F"),
			AccentBg: lipgloss.Color("#2A0845"),
			Text:     lipgloss.Color("#00F5FF"),
			Muted:    lipgloss.Color("#9D4EDD"),
			Border:   lipgloss.Color("#5B3256"),
			SelBg:    lipgloss.Color("#FF007F"),
			InBg:     lipgloss.Color("#16001E"),
			OnAccent: lipgloss.Color("#FFFFFF"),
			Green:    lipgloss.Color("#00FF41"),
			Red:      lipgloss.Color("#FF3A6A"),
			Yellow:   lipgloss.Color("#FFD700"),
			Sender: []color.Color{
				lipgloss.Color("#FF007F"), lipgloss.Color("#00F5FF"),
				lipgloss.Color("#FFD700"), lipgloss.Color("#39FF14"),
				lipgloss.Color("#BF00FF"), lipgloss.Color("#FF5F00"),
			},
		},
		Glyphs: Glyphs{"●", "o", "•", "▸", "✦", "┃", "❯", "·"},
		IsDark: true,
	},
	"nord": {
		Name: "Nord",
		Palette: Palette{
			Accent:   lipgloss.Color("#88C0D0"),
			AccentBg: lipgloss.Color("#3B4252"),
			Text:     lipgloss.Color("#ECEFF4"),
			Muted:    lipgloss.Color("#4C566A"),
			Border:   lipgloss.Color("#4C566A"),
			SelBg:    lipgloss.Color("#88C0D0"),
			InBg:     lipgloss.Color("#2E3440"),
			OnAccent: lipgloss.Color("#ECEFF4"),
			Green:    lipgloss.Color("#9ECE6A"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#88C0D0"), lipgloss.Color("#81A1C1"),
				lipgloss.Color("#B48EAD"), lipgloss.Color("#A3BE8C"),
				lipgloss.Color("#EBCB8B"), lipgloss.Color("#D08770"),
			},
		},
		Glyphs: Glyphs{"●", "o", "-", "=", "≡", "┃", "›", "·"},
		IsDark: true,
	},
	"gruvbox-amber": {
		Name: "Gruvbox Amber",
		Palette: Palette{
			Accent:   lipgloss.Color("#FE8019"),
			AccentBg: lipgloss.Color("#3C3836"),
			Text:     lipgloss.Color("#EBDBB2"),
			Muted:    lipgloss.Color("#665C54"),
			Border:   lipgloss.Color("#665C54"),
			SelBg:    lipgloss.Color("#1D2021"),
			InBg:     lipgloss.Color("#1D2021"),
			OnAccent: lipgloss.Color("#EBDBB2"),
			Green:    lipgloss.Color("#9ECE6A"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#FB4934"), lipgloss.Color("#B8BB26"),
				lipgloss.Color("#FABD2F"), lipgloss.Color("#83A598"),
				lipgloss.Color("#D3869B"), lipgloss.Color("#8EC07C"),
			},
		},
		Glyphs: Glyphs{"●", "o", "v", "vv", "√√", "┃", ">", "·"},
		IsDark: true,
	},
	"monokai-pro": {
		Name: "Monokai Pro",
		Palette: Palette{
			Accent:   lipgloss.Color("#FF6188"),
			AccentBg: lipgloss.Color("#3A3D41"),
			Text:     lipgloss.Color("#FCFCFA"),
			Muted:    lipgloss.Color("#525252"),
			Border:   lipgloss.Color("#525252"),
			SelBg:    lipgloss.Color("#2D2A2E"),
			InBg:     lipgloss.Color("#2D2A2E"),
			OnAccent: lipgloss.Color("#FFFFFF"),
			Green:    lipgloss.Color("#9ECE6A"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#FF6188"), lipgloss.Color("#FC9867"),
				lipgloss.Color("#FFD866"), lipgloss.Color("#A9DC76"),
				lipgloss.Color("#78DCE8"), lipgloss.Color("#AB9DF2"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "★", "┃", "λ", "·"},
		IsDark: true,
	},
	"solarized-light": {
		Name: "Solarized Light",
		Palette: Palette{
			Accent:   lipgloss.Color("#268BD2"),
			AccentBg: lipgloss.Color("#EEE8D5"),
			Text:     lipgloss.Color("#073642"),
			Muted:    lipgloss.Color("#586E75"),
			Border:   lipgloss.Color("#93A1A1"),
			SelBg:    lipgloss.Color("#FDF6E3"),
			InBg:     lipgloss.Color("#FDF6E3"),
			OnAccent: lipgloss.Color("#073642"),
			Green:    lipgloss.Color("#9ECE6A"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#B58900"), lipgloss.Color("#CB4B16"),
				lipgloss.Color("#DC322F"), lipgloss.Color("#D33682"),
				lipgloss.Color("#6C71C4"), lipgloss.Color("#2AA198"),
			},
		},
		Glyphs: Glyphs{"●", "o", "+", "++", "✓", "┃", "»", "·"},
		IsDark: false,
	},
	"dracula": {
		Name: "Dracula",
		Palette: Palette{
			Accent:   lipgloss.Color("#BD93F9"),
			AccentBg: lipgloss.Color("#44475A"),
			Text:     lipgloss.Color("#F8F8F2"),
			Muted:    lipgloss.Color("#6272A4"),
			Border:   lipgloss.Color("#6272A4"),
			SelBg:    lipgloss.Color("#282A36"),
			InBg:     lipgloss.Color("#282A36"),
			OnAccent: lipgloss.Color("#F8F8F2"),
			Green:    lipgloss.Color("#9ECE6A"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#FF79C6"), lipgloss.Color("#8BE9FD"),
				lipgloss.Color("#50FA7B"), lipgloss.Color("#F1FA8C"),
				lipgloss.Color("#FFB86C"), lipgloss.Color("#BD93F9"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✦", "┃", "⚡", "·"},
		IsDark: true,
	},
}

// ThemeForName resolves a named theme into a fully built Theme.
func ThemeForName(name string) Theme {
	c, ok := NamedThemes[name]
	if !ok {
		return NewTheme(colorprofile.Detect(os.Stdout, os.Environ()), true)
	}
	return buildTheme(c.Palette, ModeRich, c.Glyphs)
}
