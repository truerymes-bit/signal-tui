package main

import (
	"image/color"
	"math"
	"testing"
)

func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	toLinear := func(v uint32) float64 {
		f := float64(v) / 65535.0
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*toLinear(r) + 0.7152*toLinear(g) + 0.0722*toLinear(b)
}

func contrastBetween(fg, bg color.Color) float64 {
	l1 := relativeLuminance(fg)
	l2 := relativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func TestCyberpunkContrast(t *testing.T) {
	th := ThemeForName("cyberpunk")
	checks := []struct {
		name string
		fg   color.Color
		bg   color.Color
		min  float64
	}{
		{"Text on Bg", th.P.Text, th.P.Bg, 8.0},
		{"Text on InBg", th.P.Text, th.P.InBg, 8.0},
		{"OnAccent on AccentBg", th.P.OnAccent, th.P.AccentBg, 5.0},
		{"InBg vs Bg", th.P.InBg, th.P.Bg, 1.3},
	}
	for _, c := range checks {
		ratio := contrastBetween(c.fg, c.bg)
		if ratio < c.min {
			t.Errorf("%s: contrast %.2f < %.1f", c.name, ratio, c.min)
		} else {
			t.Logf("%s: %.2f", c.name, ratio)
		}
	}
}
