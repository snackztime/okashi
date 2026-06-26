package textarea

import "testing"

func TestMoveToLine(t *testing.T) {
	m := New()
	m.SetValue("a\nb\nc\nd\ne")
	m.MoveToLine(3)
	if m.Line() != 3 {
		t.Fatalf("MoveToLine(3) -> Line() = %d, want 3", m.Line())
	}
	m.MoveToLine(99)
	if m.Line() != 4 {
		t.Fatalf("MoveToLine(99) should clamp to the last line 4, got %d", m.Line())
	}
	m.MoveToLine(-1)
	if m.Line() != 0 {
		t.Fatalf("MoveToLine(-1) should clamp to 0, got %d", m.Line())
	}
}
