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
	Bg, Accent, AccentBg, Text, Muted, Border, SelBg, InBg color.Color
	OnAccent, Tick, Green, Red, Yellow                     color.Color
	Sender                                                 []color.Color
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
		Bg:       ld(lipgloss.Color("#F0F0F0"), lipgloss.Color("#1A1A2E")),
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
	if p.Tick == nil {
		p.Tick = p.Muted
	}
	t.P = p
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
		t.MsgIn = lipgloss.NewStyle().Foreground(p.Text).Background(p.InBg).PaddingLeft(1)
		t.MsgOut = lipgloss.NewStyle().Foreground(p.Accent).PaddingRight(1)
		t.StatusBar = lipgloss.NewStyle().Reverse(true).Padding(0, 1)
	}
	return t
}

var spaceRunRE = regexp.MustCompile(` {2,}`)

func (t Theme) Base() lipgloss.Style {
	return lipgloss.NewStyle().Background(t.P.Bg).Foreground(t.P.Text)
}

func (t Theme) Status(w int, left, right string) string {
	if lipgloss.Width(right) > w-4 && w-4 > 0 {
		right = truncateToWidth(right, w-4)
	}
	maxLeft := w - 2 - lipgloss.Width(right) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = truncateToWidth(left, maxLeft)
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
			Bg:       lipgloss.Color("#0c0b08"),
			InBg:     lipgloss.Color("#2a271d"),
			Border:   lipgloss.Color("#45402c"),
			Text:     lipgloss.Color("#c4ddd7"),
			Muted:    lipgloss.Color("#939f98"),
			Accent:   lipgloss.Color("#d6c55a"),
			AccentBg: lipgloss.Color("#6a5f1f"),
			OnAccent: lipgloss.Color("#f6f0cf"),
			SelBg:    lipgloss.Color("#26403f"),
			Tick:     lipgloss.Color("#86c9bd"),
			Green:    lipgloss.Color("#79b79a"),
			Red:      lipgloss.Color("#dc6e7a"),
			Yellow:   lipgloss.Color("#d6c55a"),
			Sender: []color.Color{
				lipgloss.Color("#d6c55a"), lipgloss.Color("#6fc3c0"),
				lipgloss.Color("#d8727e"), lipgloss.Color("#a99bd0"),
				lipgloss.Color("#d4955a"), lipgloss.Color("#93c283"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "❯", "·"},
		IsDark: true,
	},
	"nord": {
		Name: "Nord",
		Palette: Palette{
			Accent:   lipgloss.Color("#88C0D0"),
			AccentBg: lipgloss.Color("#3B4252"),
			Text:     lipgloss.Color("#ECEFF4"),
			Muted:    lipgloss.Color("#999fac"),
			Tick:     lipgloss.Color("#afb5c0"),
			Border:   lipgloss.Color("#4C566A"),
			SelBg:    lipgloss.Color("#88C0D0"),
			InBg:     lipgloss.Color("#2E3440"),
			Bg:       lipgloss.Color("#1d2028"),
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
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "›", "·"},
		IsDark: true,
	},
	"gruvbox-amber": {
		Name: "Gruvbox Amber",
		Palette: Palette{
			Accent:   lipgloss.Color("#FE8019"),
			AccentBg: lipgloss.Color("#3C3836"),
			Text:     lipgloss.Color("#EBDBB2"),
			Muted:    lipgloss.Color("#a3967f"),
			Tick:     lipgloss.Color("#b8aa8e"),
			Border:   lipgloss.Color("#665C54"),
			SelBg:    lipgloss.Color("#1D2021"),
			InBg:     lipgloss.Color("#292e2f"),
			Bg:       lipgloss.Color("#151718"),
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
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", ">", "·"},
		IsDark: true,
	},
	"monokai-pro": {
		Name: "Monokai Pro",
		Palette: Palette{
			Accent:   lipgloss.Color("#FF6188"),
			AccentBg: lipgloss.Color("#3A3D41"),
			Text:     lipgloss.Color("#FCFCFA"),
			Muted:    lipgloss.Color("#959594"),
			Tick:     lipgloss.Color("#a9a9a8"),
			Border:   lipgloss.Color("#525252"),
			SelBg:    lipgloss.Color("#2D2A2E"),
			InBg:     lipgloss.Color("#2D2A2E"),
			Bg:       lipgloss.Color("#141214"),
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
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "λ", "·"},
		IsDark: true,
	},
	"solarized-light": {
		Name: "Solarized Light",
		Palette: Palette{
			Bg:       lipgloss.Color("#FDF6E3"),
			Accent:   lipgloss.Color("#268BD2"),
			AccentBg: lipgloss.Color("#D4E3EE"),
			Text:     lipgloss.Color("#073642"),
			Muted:    lipgloss.Color("#446069"),
			Tick:     lipgloss.Color("#207c75"),
			Border:   lipgloss.Color("#93A1A1"),
			SelBg:    lipgloss.Color("#D4E3EE"),
			InBg:     lipgloss.Color("#ded9c7"),
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
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "»", "·"},
		IsDark: false,
	},
	"dusk-slate": {
		Name: "Dusk Slate",
		Palette: Palette{
			Accent:   lipgloss.Color("#7f9fd6"),
			AccentBg: lipgloss.Color("#3d5a8c"),
			Text:     lipgloss.Color("#c8cfdf"),
			Muted:    lipgloss.Color("#939bb2"),
			Tick:     lipgloss.Color("#a8b0c4"),
			Border:   lipgloss.Color("#3a4258"),
			SelBg:    lipgloss.Color("#2a3350"),
			InBg:     lipgloss.Color("#2a3045"),
			Bg:       lipgloss.Color("#181b27"),
			OnAccent: lipgloss.Color("#e3e8f3"),
			Green:    lipgloss.Color("#9aa5c0"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#7f9fd6"), lipgloss.Color("#9aa5c0"),
				lipgloss.Color("#3d5a8c"), lipgloss.Color("#c8cfdf"),
				lipgloss.Color("#e3e8f3"), lipgloss.Color("#6b7490"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "❯", "·"},
		IsDark: true,
	},
	"moss": {
		Name: "Moss",
		Palette: Palette{
			Accent:   lipgloss.Color("#8fb89a"),
			AccentBg: lipgloss.Color("#3f6350"),
			Text:     lipgloss.Color("#c9d4cb"),
			Muted:    lipgloss.Color("#919f95"),
			Tick:     lipgloss.Color("#9db3a4"),
			Border:   lipgloss.Color("#37443d"),
			SelBg:    lipgloss.Color("#2a3a31"),
			InBg:     lipgloss.Color("#28332d"),
			Bg:       lipgloss.Color("#171d1a"),
			OnAccent: lipgloss.Color("#e2ece5"),
			Green:    lipgloss.Color("#9db3a4"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#8fb89a"), lipgloss.Color("#9db3a4"),
				lipgloss.Color("#3f6350"), lipgloss.Color("#c9d4cb"),
				lipgloss.Color("#e2ece5"), lipgloss.Color("#68776d"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "❯", "·"},
		IsDark: true,
	},
	"warm-sepia": {
		Name: "Warm Sepia",
		Palette: Palette{
			Accent:   lipgloss.Color("#d1a67a"),
			AccentBg: lipgloss.Color("#7a5a3e"),
			Text:     lipgloss.Color("#d6cbbf"),
			Muted:    lipgloss.Color("#a19488"),
			Tick:     lipgloss.Color("#b5a99c"),
			Border:   lipgloss.Color("#453d38"),
			SelBg:    lipgloss.Color("#38302b"),
			InBg:     lipgloss.Color("#322b27"),
			Bg:       lipgloss.Color("#181413"),
			OnAccent: lipgloss.Color("#f0e6da"),
			Green:    lipgloss.Color("#b3a493"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#d1a67a"), lipgloss.Color("#b3a493"),
				lipgloss.Color("#7a5a3e"), lipgloss.Color("#d6cbbf"),
				lipgloss.Color("#f0e6da"), lipgloss.Color("#8a7d70"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "❯", "·"},
		IsDark: true,
	},
	"lavender-ink": {
		Name: "Lavender Ink",
		Palette: Palette{
			Accent:   lipgloss.Color("#a99bd0"),
			AccentBg: lipgloss.Color("#5a4f8a"),
			Text:     lipgloss.Color("#cfcbe0"),
			Muted:    lipgloss.Color("#9794ae"),
			Tick:     lipgloss.Color("#aca9c1"),
			Border:   lipgloss.Color("#40405a"),
			SelBg:    lipgloss.Color("#322f4a"),
			InBg:     lipgloss.Color("#2d2a42"),
			Bg:       lipgloss.Color("#161420"),
			OnAccent: lipgloss.Color("#ebe7f5"),
			Green:    lipgloss.Color("#a39fbd"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#a99bd0"), lipgloss.Color("#a39fbd"),
				lipgloss.Color("#5a4f8a"), lipgloss.Color("#cfcbe0"),
				lipgloss.Color("#ebe7f5"), lipgloss.Color("#767391"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "❯", "·"},
		IsDark: true,
	},
	"teal-fog": {
		Name: "Teal Fog",
		Palette: Palette{
			Accent:   lipgloss.Color("#7fb5b5"),
			AccentBg: lipgloss.Color("#3a6a6e"),
			Text:     lipgloss.Color("#c5d3d4"),
			Muted:    lipgloss.Color("#8da0a3"),
			Tick:     lipgloss.Color("#a4b5b7"),
			Border:   lipgloss.Color("#34464b"),
			SelBg:    lipgloss.Color("#27393f"),
			InBg:     lipgloss.Color("#26343a"),
			Bg:       lipgloss.Color("#171f23"),
			OnAccent: lipgloss.Color("#e0eeee"),
			Green:    lipgloss.Color("#96b0b2"),
			Red:      lipgloss.Color("#F7768E"),
			Yellow:   lipgloss.Color("#E0AF68"),
			Sender: []color.Color{
				lipgloss.Color("#7fb5b5"), lipgloss.Color("#96b0b2"),
				lipgloss.Color("#3a6a6e"), lipgloss.Color("#c5d3d4"),
				lipgloss.Color("#e0eeee"), lipgloss.Color("#647a7e"),
			},
		},
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "❯", "·"},
		IsDark: true,
	},
	"dracula": {
		Name: "Dracula",
		Palette: Palette{
			Accent:   lipgloss.Color("#BD93F9"),
			AccentBg: lipgloss.Color("#44475A"),
			Text:     lipgloss.Color("#F8F8F2"),
			Muted:    lipgloss.Color("#8894b8"),
			Tick:     lipgloss.Color("#9fa8c4"),
			Border:   lipgloss.Color("#6272A4"),
			SelBg:    lipgloss.Color("#282A36"),
			InBg:     lipgloss.Color("#282A36"),
			Bg:       lipgloss.Color("#121319"),
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
		Glyphs: Glyphs{"●", "o", "✓", "✓✓", "✓✓", "┃", "⚡", "·"},
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
