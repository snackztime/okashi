package textarea

import "testing"

func TestReplaceRange(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(4)
	m.SetValue("the cat")
	// cursor is on the only line (row 0); replace "cat" [4,7) with "dog".
	m.ReplaceRange(4, 7, "dog")
	if got := m.Value(); got != "the dog" {
		t.Fatalf("Value = %q, want \"the dog\"", got)
	}
	if got := m.CursorColumn(); got != 7 {
		t.Fatalf("CursorColumn = %d, want 7 (after \"dog\")", got)
	}
}

func TestReplaceRangeMultibyte(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(4)
	m.SetValue("café teh")
	// "teh" is runes [5,8) (é is one rune); replace with "tea".
	m.ReplaceRange(5, 8, "tea")
	if got := m.Value(); got != "café tea" {
		t.Fatalf("Value = %q, want \"café tea\"", got)
	}
}
