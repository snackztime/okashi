package textarea

import "testing"

func TestIndentOutdent(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(5) // end of "hello"

	m.Indent()
	if got := m.Value(); got != "  hello" {
		t.Fatalf("after Indent: %q, want %q", got, "  hello")
	}
	if m.col != 7 {
		t.Fatalf("cursor col = %d, want 7", m.col)
	}

	m.Outdent()
	if got := m.Value(); got != "hello" {
		t.Fatalf("after Outdent: %q, want %q", got, "hello")
	}
	if m.col != 5 {
		t.Fatalf("cursor col = %d, want 5", m.col)
	}

	m.Outdent() // no leading spaces → no-op
	if got := m.Value(); got != "hello" {
		t.Fatalf("Outdent on unindented line changed it: %q", got)
	}
}

func TestLineHelpers(t *testing.T) {
	m := New()
	m.SetValue("- item")
	m.SetCursor(6)
	if m.CurrentLine() != "- item" {
		t.Fatalf("CurrentLine = %q", m.CurrentLine())
	}
	if !m.AtLineEnd() {
		t.Fatal("AtLineEnd should be true at col 6")
	}
	if r, ok := m.CharBeforeCursor(); !ok || r != 'm' {
		t.Fatalf("CharBeforeCursor = %q,%v want 'm',true", r, ok)
	}
	m.SetCursor(0)
	if _, ok := m.CharBeforeCursor(); ok {
		t.Fatal("CharBeforeCursor at col 0 should be ok=false")
	}
	if m.AtLineEnd() {
		t.Fatal("AtLineEnd should be false at col 0 of a non-empty line")
	}

	m.ClearLine()
	if m.Value() != "" || m.col != 0 {
		t.Fatalf("after ClearLine: %q col=%d", m.Value(), m.col)
	}
}
