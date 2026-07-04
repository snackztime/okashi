package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reconstruct rebuilds a (equal+del) and b (equal+add) from an edit script — the invariant a
// correct diff must satisfy.
func reconstruct(ops []diffLine) (a, b []string) {
	for _, op := range ops {
		switch op.op {
		case opEqual:
			a = append(a, op.text)
			b = append(b, op.text)
		case opDel:
			a = append(a, op.text)
		case opAdd:
			b = append(b, op.text)
		}
	}
	return a, b
}

func joinEq(s []string) string { return strings.Join(s, "\n") }

func checkDiff(t *testing.T, a, b []string) []diffLine {
	t.Helper()
	ops := diffLines(a, b)
	ra, rb := reconstruct(ops)
	if joinEq(ra) != joinEq(a) {
		t.Fatalf("reconstruct a: got %v, want %v", ra, a)
	}
	if joinEq(rb) != joinEq(b) {
		t.Fatalf("reconstruct b: got %v, want %v", rb, b)
	}
	return ops
}

func countOp(ops []diffLine, op diffOp) int {
	n := 0
	for _, o := range ops {
		if o.op == op {
			n++
		}
	}
	return n
}

func TestDiffLinesIdentical(t *testing.T) {
	a := []string{"one", "two", "three"}
	ops := checkDiff(t, a, a)
	if countOp(ops, opDel) != 0 || countOp(ops, opAdd) != 0 {
		t.Fatalf("identical inputs should have no del/add: %+v", ops)
	}
	if countOp(ops, opEqual) != 3 {
		t.Fatalf("expected 3 equal lines, got %+v", ops)
	}
}

func TestDiffLinesPureInsert(t *testing.T) {
	ops := checkDiff(t, []string{"one", "three"}, []string{"one", "two", "three"})
	if countOp(ops, opAdd) != 1 || countOp(ops, opDel) != 0 {
		t.Fatalf("expected exactly one add: %+v", ops)
	}
}

func TestDiffLinesPureDelete(t *testing.T) {
	ops := checkDiff(t, []string{"one", "two", "three"}, []string{"one", "three"})
	if countOp(ops, opDel) != 1 || countOp(ops, opAdd) != 0 {
		t.Fatalf("expected exactly one delete: %+v", ops)
	}
}

func TestDiffLinesReplaceAndInterleave(t *testing.T) {
	checkDiff(t, []string{"a", "b", "c"}, []string{"a", "X", "c"})
	checkDiff(t, []string{"a", "b", "c", "d", "e"}, []string{"a", "c", "x", "e", "f"})
	checkDiff(t, nil, []string{"only", "adds"})
	checkDiff(t, []string{"only", "dels"}, nil)
	checkDiff(t, nil, nil)
}

func TestDiffWordsHighlightsOnlyChanged(t *testing.T) {
	aRuns, bRuns := diffWords("the quick brown fox", "the slow brown fox")
	// The b side should mark "slow" changed and leave "brown"/"fox"/"the" unchanged.
	var changedB []string
	var rebuiltB strings.Builder
	for _, r := range bRuns {
		rebuiltB.WriteString(r.text)
		if r.changed && strings.TrimSpace(r.text) != "" {
			changedB = append(changedB, strings.TrimSpace(r.text))
		}
	}
	if rebuiltB.String() != "the slow brown fox" {
		t.Fatalf("b runs must rejoin to the original, got %q", rebuiltB.String())
	}
	if len(changedB) != 1 || changedB[0] != "slow" {
		t.Fatalf("only 'slow' should be highlighted on the b side, got %v", changedB)
	}
	// The a side should highlight "quick".
	var changedA []string
	for _, r := range aRuns {
		if r.changed && strings.TrimSpace(r.text) != "" {
			changedA = append(changedA, strings.TrimSpace(r.text))
		}
	}
	if len(changedA) != 1 || changedA[0] != "quick" {
		t.Fatalf("only 'quick' should be highlighted on the a side, got %v", changedA)
	}
}

func TestDiffModelWindowsAndHighlights(t *testing.T) {
	a := "line one\nthe quick brown fox\nline three"
	b := "line one\nthe slow brown fox\nline three\nadded"
	d := newDiffModel("old", a, "new", b)
	if len(d.wordRuns) == 0 {
		t.Fatal("expected word-level highlights for the changed line pair")
	}
	m := model{width: 80, height: 10, screen: screenDiff, diff: d}
	out := m.diffView()
	if n := strings.Count(out, "\n"); n > 12 {
		t.Fatalf("diff view should window to ~height rows, got %d newlines", n)
	}
	if !strings.Contains(out, "diff · old → new") {
		t.Fatalf("diff header missing from view")
	}
}

func TestDiffJumpChange(t *testing.T) {
	d := newDiffModel("a", "x\ny\nz", "b", "x\nY\nz")
	if got := d.jumpChange(0, 1, 100); got <= 0 {
		t.Fatalf("jumpChange forward should advance to a change, got %d", got)
	}
}

func TestDiffEntrySnapshotVsCurrent(t *testing.T) {
	file, bak := seedSnapshots(t) // file content = "CURRENT"
	if err := os.WriteFile(filepath.Join(bak, "a.md.20260704-101500"), []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := model{snapshots: newSnapshotsModel(file)}
	m.snapshots.sel = 0
	m.openDiffSnapshotVsCurrent()

	if m.screen != screenDiff {
		t.Fatal("should switch to the diff screen")
	}
	if m.diff.bLabel != "current" {
		t.Fatalf("bLabel = %q, want current", m.diff.bLabel)
	}
	var del, add bool
	for _, l := range m.diff.lines {
		if l.op == opDel && l.text == "OLD" {
			del = true
		}
		if l.op == opAdd && l.text == "CURRENT" {
			add = true
		}
	}
	if !del || !add {
		t.Fatalf("diff should show OLD removed, CURRENT added (del=%v add=%v)", del, add)
	}
}
