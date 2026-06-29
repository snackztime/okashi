package textarea

import "testing"

func TestClickToPlainLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("alpha bravo charlie")
	// Click on display row 0, col 8 (inside "bravo", which is runes 6..11).
	m.ClickTo(0, 8)
	if r, c := m.Line(), m.CursorColumn(); r != 0 || c != 8 {
		t.Fatalf("ClickTo(0,8) → row %d col %d, want 0,8", r, c)
	}
}

func TestClickToClampsPastEnd(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("short")
	m.ClickTo(0, 30) // past the end of the line
	if c := m.CursorColumn(); c != 5 {
		t.Fatalf("ClickTo past line end → col %d, want 5 (line length)", c)
	}
}

func TestClickToSecondLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("first\nsecond line\nthird")
	m.ClickTo(1, 3) // row 1, col 3 → inside "second"
	if r, c := m.Line(), m.CursorColumn(); r != 1 || c != 3 {
		t.Fatalf("ClickTo(1,3) → row %d col %d, want 1,3", r, c)
	}
}

func TestClickToMultibyte(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("café crème brûlée") // multibyte; runes, not bytes
	m.ClickTo(0, 6)                 // inside "crème" (rune 5..10)
	if c := m.CursorColumn(); c < 5 || c > 10 {
		t.Fatalf("ClickTo on a multibyte line → col %d, want within [5,10]", c)
	}
}
