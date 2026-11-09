package main

import (
	"image/color"
	"math"
	"testing"
)

func lum(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	toLin := func(v uint32) float64 {
		f := float64(v) / 65535.0
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*toLin(r) + 0.7152*toLin(g) + 0.0722*toLin(b)
}

func ratio(fg, bg color.Color) float64 {
	l1, l2 := lum(fg), lum(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func TestThemeContrastThresholds(t *testing.T) {
	for _, name := range namedThemeKeys() {
		th := ThemeForName(name)
		cfg := NamedThemes[name]
		// InBg vs Bg >= 1.3
		r := ratio(th.P.InBg, th.P.Bg)
		t.Logf("%s: InBg/Bg=%.4f", name, r)
		if r < 1.3 {
			t.Errorf("%s: InBg/Bg contrast %.2f < 1.3", name, r)
		}
		if cfg.IsDark {
			r1 := ratio(th.P.Text, th.P.Bg)
			r2 := ratio(th.P.Text, th.P.InBg)
			if r1 < 8.0 {
				t.Errorf("%s: Text/Bg contrast %.2f < 8", name, r1)
			}
			if r2 < 8.0 {
				t.Errorf("%s: Text/InBg contrast %.2f < 8", name, r2)
			}
			t.Logf("%s: Text/Bg=%.2f Text/InBg=%.2f", name, r1, r2)
		} else {
			// solarized-light report only
			rIn := ratio(th.P.Text, th.P.InBg)
			rBg := ratio(th.P.Text, th.P.Bg)
			t.Logf("%s: Text/InBg=%.2f Text/Bg=%.2f", name, rIn, rBg)
		}
	}
}
