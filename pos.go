package main

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdkato/prose/v2"
	"okashi/internal/textarea"
)

type posToken struct {
	text, tag  string
	start, end int // rune offsets
}

var (
	adverbStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")) // yellow
	adjStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")) // cyan
	passiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb86c")) // orange
)

var (
	posMu    sync.Mutex
	posCache = map[string][]posToken{}
)

// posTokens tags a line with prose, mapping tokens to RUNE offsets. Memoized:
// the Decorator runs per visible line each frame, but lines change only on edit.
func posTokens(line string) []posToken {
	posMu.Lock()
	if t, ok := posCache[line]; ok {
		posMu.Unlock()
		return t
	}
	posMu.Unlock()

	toks := tagLine(line)

	posMu.Lock()
	if len(posCache) > 4096 {
		posCache = map[string][]posToken{}
	}
	posCache[line] = toks
	posMu.Unlock()
	return toks
}

func tagLine(line string) []posToken {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	doc, err := prose.NewDocument(line, prose.WithExtraction(false), prose.WithSegmentation(false))
	if err != nil {
		return nil
	}
	var toks []posToken
	byteCursor := 0
	for _, tk := range doc.Tokens() {
		idx := strings.Index(line[byteCursor:], tk.Text)
		if idx < 0 {
			continue
		}
		bs := byteCursor + idx
		be := bs + len(tk.Text)
		toks = append(toks, posToken{
			text:  tk.Text,
			tag:   tk.Tag,
			start: len([]rune(line[:bs])),
			end:   len([]rune(line[:be])),
		})
		byteCursor = be
	}
	return toks
}

func isBeVerb(s string) bool {
	switch strings.ToLower(s) {
	case "am", "is", "are", "was", "were", "be", "been", "being":
		return true
	}
	return false
}

// posDecorator styles the active POS categories on one line.
func posDecorator(line string, adverb, adjective, passive bool) []textarea.Decoration {
	if !adverb && !adjective && !passive {
		return nil
	}
	toks := posTokens(line)
	var decos []textarea.Decoration
	add := func(t posToken, style lipgloss.Style) {
		decos = append(decos, textarea.Decoration{Start: t.start, End: t.end, Style: style})
	}
	for i, t := range toks {
		if adverb && strings.HasPrefix(t.tag, "RB") {
			add(t, adverbStyle)
		}
		if adjective && strings.HasPrefix(t.tag, "JJ") {
			add(t, adjStyle)
		}
		if passive && isBeVerb(t.text) {
			add(t, passiveStyle)
			for j := i + 1; j < len(toks); j++ {
				if strings.HasPrefix(toks[j].tag, "RB") {
					continue // skip an adverb between "was" and the participle
				}
				if toks[j].tag == "VBN" {
					add(toks[j], passiveStyle)
				}
				break
			}
		}
	}
	return decos
}
