package textarea

import "testing"

func TestCurrentLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(5)
	m.SetValue("first line\nsecond line\nthird")
	m.MoveToLine(1)
	if got := m.CurrentLine(); got != "second line" {
		t.Fatalf("CurrentLine = %q, want \"second line\"", got)
	}
	m.MoveToLine(2)
	if got := m.CurrentLine(); got != "third" {
		t.Fatalf("CurrentLine = %q, want \"third\"", got)
	}
}
