package main

import (
	"testing"
)

func TestMutedAndTickContrast(t *testing.T) {
	for _, name := range namedThemeKeys() {
		th := ThemeForName(name)
		cfg := NamedThemes[name]
		// Muted legible on both backgrounds.
		rIn := ratio(th.P.Muted, th.P.InBg)
		rBg := ratio(th.P.Muted, th.P.Bg)
		if rIn < 4.5 {
			t.Errorf("%s: Muted/InBg contrast %.2f < 4.5", name, rIn)
		}
		if rBg < 4.5 {
			t.Errorf("%s: Muted/Bg contrast %.2f < 4.5", name, rBg)
		}
		// Tick must be set explicitly in config (not relying on fallback).
		if cfg.Palette.Tick == nil {
			t.Errorf("%s: Tick not set explicitly", name)
		}
		rTickBg := ratio(th.P.Tick, th.P.Bg)
		if rTickBg < 4.5 {
			t.Errorf("%s: Tick/Bg contrast %.2f < 4.5", name, rTickBg)
		}
		// Tick legible on InBg for dark themes where InBg is lighter than Bg.
		if lum(th.P.InBg) >= lum(th.P.Bg) {
			rTickIn := ratio(th.P.Tick, th.P.InBg)
			if rTickIn < 5.5 {
				t.Errorf("%s: Tick/InBg contrast %.2f < 5.5", name, rTickIn)
			}
			t.Logf("%s: Muted/InBg=%.2f Muted/Bg=%.2f Tick/Bg=%.2f Tick/InBg=%.2f", name, rIn, rBg, rTickBg, rTickIn)
		} else {
			t.Logf("%s: Muted/InBg=%.2f Muted/Bg=%.2f Tick/Bg=%.2f", name, rIn, rBg, rTickBg)
		}
	}
}
