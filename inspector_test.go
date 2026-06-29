package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestComputeDocStats(t *testing.T) {
	ds := computeDocStats("Hello world.\n\nSecond paragraph here.\n")
	if ds.words != 5 {
		t.Fatalf("words = %d, want 5", ds.words)
	}
	if ds.paragraphs != 2 {
		t.Fatalf("paragraphs = %d, want 2", ds.paragraphs)
	}
	if ds.chars == 0 {
		t.Fatal("chars should be non-zero")
	}
	// Trailing blank lines must not inflate the paragraph count.
	if got := computeDocStats("One.\n\n\n\nTwo.\n\n").paragraphs; got != 2 {
		t.Fatalf("paragraphs with extra blank lines = %d, want 2", got)
	}
	// Empty buffer → all zero.
	if z := computeDocStats(""); z.words != 0 || z.chars != 0 || z.paragraphs != 0 {
		t.Fatalf("empty stats = %+v, want zero", z)
	}
}

func TestComputeProjStatsManuscript(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644) // 3
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)     // 2
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("loose note words"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	ps := computeProjStats(dir, v, newWordCountCache())
	if !ps.manuscript {
		t.Fatal("expected manuscript = true (numbered chapters present)")
	}
	if ps.chapters != 2 {
		t.Fatalf("chapters = %d, want 2", ps.chapters)
	}
	if ps.words != 5 {
		t.Fatalf("project words = %d, want 5 (chapters only, loose excluded)", ps.words)
	}
}

func TestComputeProjStatsPlainFolder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("three"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	ps := computeProjStats(dir, v, newWordCountCache())
	if ps.manuscript {
		t.Fatal("plain folder must not be a manuscript")
	}
	if ps.words != 3 {
		t.Fatalf("folder words = %d, want 3 (sum of loose docs)", ps.words)
	}
}

func TestInspectorViewRendersWords(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{words: 1204, chars: 6830, paragraphs: 38}, projStats{words: 47032, chapters: 12, manuscript: true}, "", goalStats{}, analysisState{})
	for _, want := range []string{"Words", "Document", "Project", "1,204", "47,032", "Chapters", "12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspector view missing %q:\n%s", want, out)
		}
	}
	// Non-manuscript omits the Chapters line.
	plain := in.View(28, docStats{words: 10}, projStats{words: 10, manuscript: false}, "", goalStats{}, analysisState{})
	if strings.Contains(plain, "Chapters") {
		t.Fatal("non-manuscript inspector should omit 'Chapters'")
	}
}

func TestInspectorCycle(t *testing.T) {
	in := inspectorModel{}
	in.cycle()
	if !in.visible || in.tab != tabWords {
		t.Fatalf("first cycle: visible=%v tab=%v, want visible Words", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabOutline {
		t.Fatalf("second cycle: visible=%v tab=%v, want visible Outline", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabGoals {
		t.Fatalf("third cycle: visible=%v tab=%v, want visible Goals", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabAnalysis {
		t.Fatalf("fourth cycle: visible=%v tab=%v, want visible Analysis", in.visible, in.tab)
	}
	in.cycle()
	if in.visible {
		t.Fatal("fifth cycle should close the inspector (past the last tab)")
	}
	if in.tab != tabWords {
		t.Fatalf("closed cycle should reset tab to Words, got %v", in.tab)
	}
}

func TestInspectorOutlineTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabOutline}
	out := in.View(28, docStats{}, projStats{}, "- Top\n  - sub", goalStats{}, analysisState{})
	for _, want := range []string{"Outline", "Top", "sub"} {
		if !strings.Contains(out, want) {
			t.Fatalf("outline tab missing %q:\n%s", want, out)
		}
	}
	empty := in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{})
	if !strings.Contains(empty, "empty") {
		t.Fatal("empty outline should show an (empty …) hint")
	}
}

func TestProgressBar(t *testing.T) {
	if b := progressBar(0, 100, 10); strings.Count(b, "█") != 0 {
		t.Fatalf("0%% bar should have no fill: %q", b)
	}
	half := progressBar(50, 100, 10)
	if n := strings.Count(half, "█"); n < 4 || n > 6 {
		t.Fatalf("50%% bar fill = %d cells, want ~5", n)
	}
	if n := strings.Count(progressBar(200, 100, 10), "█"); n != 10 {
		t.Fatalf(">=100%% bar should be full (10), got %d", n)
	}
	if strings.Count(progressBar(5, 0, 10), "█") != 0 {
		t.Fatal("goal 0 → empty bar")
	}
}

func TestInspectorGoalsTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabGoals}
	out := in.View(28, docStats{}, projStats{}, "", goalStats{today: 312, dailyGoal: 500, project: 47032, projectGoal: 80000}, analysisState{})
	for _, w := range []string{"Daily", "312", "500", "Project", "80,000"} {
		if !strings.Contains(out, w) {
			t.Fatalf("goals tab missing %q:\n%s", w, out)
		}
	}
	// projectGoal 0 → no Project section.
	noproj := in.View(28, docStats{}, projStats{}, "", goalStats{today: 10, dailyGoal: 500, project: 10, projectGoal: 0}, analysisState{})
	if strings.Contains(noproj, "Project") {
		t.Fatal("projectGoal 0 should omit the Project section")
	}
	// goal met.
	met := in.View(28, docStats{}, projStats{}, "", goalStats{today: 600, dailyGoal: 500, project: 1, projectGoal: 0}, analysisState{})
	if !strings.Contains(met, "met") {
		t.Fatal("today >= daily goal should show '✓ goal met'")
	}
}

func TestInspectorTabAtX(t *testing.T) {
	// labels {"Words","Outline","Goals","Analysis"} → no padding, single-space separated.
	// Words(0..4) space(5) Outline(6..12) space(13) Goals(14..18) space(19) Analysis(20..27)
	if tb, ok := inspectorTabAtX(2); !ok || tb != tabWords {
		t.Fatalf("x=2 → %v ok=%v, want Words", tb, ok)
	}
	if tb, ok := inspectorTabAtX(8); !ok || tb != tabOutline {
		t.Fatalf("x=8 → %v ok=%v, want Outline", tb, ok)
	}
	if tb, ok := inspectorTabAtX(15); !ok || tb != tabGoals {
		t.Fatalf("x=15 → %v ok=%v, want Goals", tb, ok)
	}
	if tb, ok := inspectorTabAtX(22); !ok || tb != tabAnalysis {
		t.Fatalf("x=22 → %v ok=%v, want Analysis", tb, ok)
	}
	if _, ok := inspectorTabAtX(100); ok {
		t.Fatal("x past the last chip → not ok")
	}
}

func TestInspectorAnalysisTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	on := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: false})
	if !strings.Contains(on, "Spellcheck") || !strings.Contains(on, "Syntax") {
		t.Fatalf("analysis tab should list Spellcheck and Syntax:\n%s", on)
	}
	if !strings.Contains(on, "[x] Spellcheck") {
		t.Fatalf("spell on → checked box:\n%s", on)
	}
	off := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{})
	if !strings.Contains(off, "[ ] Spellcheck") {
		t.Fatalf("spell off → empty box:\n%s", off)
	}
}

func TestInspectorAnalysisRowAtY(t *testing.T) {
	// Spellcheck is at analysisRowY(0); Adverb at analysisRowY(1).
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(0)); !ok || r != 0 {
		t.Fatalf("Spellcheck row → %d ok=%v, want 0", r, ok)
	}
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(1)); !ok || r != 1 {
		t.Fatalf("Adverb row → %d ok=%v, want 1", r, ok)
	}
	if _, ok := inspectorAnalysisRowAtY(0); ok {
		t.Fatal("the tab-bar row is not a checkbox row")
	}
}

func TestAnalysisTabPOSList(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	out := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: true})
	for _, w := range []string{"Spellcheck", "Syntax", "Adverb", "Adjective", "Passive"} {
		if !strings.Contains(out, w) {
			t.Fatalf("analysis tab missing %q:\n%s", w, out)
		}
	}
	if !strings.Contains(out, "[x] Spellcheck") || !strings.Contains(out, "[x] Adverb") {
		t.Fatalf("toggled-on checkboxes should render [x]:\n%s", out)
	}
	// Verify rows render at the expected Y positions.
	lines := strings.Split(out, "\n")
	for i, label := range []string{"Spellcheck", "Adverb", "Adjective", "Passive"} {
		y := analysisRowY(i)
		if y >= len(lines) || !strings.Contains(lines[y], label) {
			t.Fatalf("row %d (analysisRowY(%d)=%d) should contain %q, got %q", i, i, y, label, func() string {
				if y < len(lines) {
					return lines[y]
				}
				return "<out of range>"
			}())
		}
	}
}

func TestTabBarFitsOneRow(t *testing.T) {
	in := inspectorModel{visible: true}
	bar := in.tabBar()
	if lipgloss.Height(bar) != 1 {
		t.Fatalf("tab bar should be one row, got %d: %q", lipgloss.Height(bar), bar)
	}
	if lipgloss.Width(bar) > inspectorInnerWidth() {
		t.Fatalf("tab bar width %d exceeds inner width %d", lipgloss.Width(bar), inspectorInnerWidth())
	}
}

func TestAnalysisRowAtY(t *testing.T) {
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(0)); !ok || r != 0 {
		t.Fatalf("Spellcheck row → %d ok=%v, want 0", r, ok)
	}
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(3)); !ok || r != 3 {
		t.Fatalf("Passive row → %d ok=%v, want 3", r, ok)
	}
	if _, ok := inspectorAnalysisRowAtY(0); ok {
		t.Fatal("the tab-bar row is not a checkbox row")
	}
}
