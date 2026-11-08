package main

import "testing"

func TestSizesInvariant(t *testing.T) {
	cases := []struct{ w, h int }{
		{240, 67}, {46, 33}, {44, 44}, {32, 9}, {33, 37}, {41, 42},
		{80, 24}, {10, 5}, {0, 0}, {1, 1}, {30, 8}, {29, 7}, {7, 10},
	}
	for _, c := range cases {
		m := model{width: c.w, height: c.h}
		sw, cw, bodyH := m.sizes()
		if sw < 1 {
			t.Errorf("w=%d h=%d: sw=%d <1", c.w, c.h, sw)
		}
		if cw < 1 {
			t.Errorf("w=%d h=%d: cw=%d <1", c.w, c.h, cw)
		}
		cw2 := contentWidth(m)
		if cw2 >= 8 && sw+cw != cw2 {
			t.Errorf("w=%d: sw+cw=%d != contentWidth=%d", c.w, sw+cw, cw2)
		}
		if bodyH < 1 {
			t.Errorf("h=%d: bodyH=%d <1", c.h, bodyH)
		}
		if c.h > 0 && bodyH > c.h {
			t.Errorf("h=%d: bodyH=%d > %d", c.h, bodyH, c.h)
		}
	}
}
