package game

import "testing"

func TestAngleSinCosQuadrants(t *testing.T) {
	cases := []struct {
		name    string
		a       Angle
		wantSin float32
		wantCos float32
	}{
		{"0", 0, 0, 1},
		{"quarter turn (1024)", 1024, 1, 0},
		{"half turn (2048)", 2048, 0, -1},
		{"three-quarter turn (3072)", 3072, -1, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			const tolerance = 1e-6
			if got := c.a.Sin(); abs32(got-c.wantSin) > tolerance {
				t.Errorf("Sin(%d) = %v, want %v", c.a, got, c.wantSin)
			}
			if got := c.a.Cos(); abs32(got-c.wantCos) > tolerance {
				t.Errorf("Cos(%d) = %v, want %v", c.a, got, c.wantCos)
			}
		})
	}
}

func TestAngleWrapped(t *testing.T) {
	if got := Angle(5000).Wrapped(); got != 904 {
		t.Errorf("Wrapped() = %d, want 904", got)
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
