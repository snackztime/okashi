package main

import (
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	snippetMaxRunes  = 80
	snippetReadBytes = 400
)

type snippetEntry struct {
	mtime int64
	text  string
}

// snippetCache returns a one-line, markdown-stripped preview of a document's opening
// prose, reading only the first ~400 bytes. Cached by path + mtime (mirrors wordCountCache).
type snippetCache struct {
	mu sync.Mutex
	m  map[string]snippetEntry
}

func newSnippetCache() *snippetCache { return &snippetCache{m: map[string]snippetEntry{}} }

func (c *snippetCache) get(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return ""
	}
	mt := fi.ModTime().UnixNano()
	c.mu.Lock()
	if e, ok := c.m[path]; ok && e.mtime == mt {
		c.mu.Unlock()
		return e.text
	}
	c.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	buf := make([]byte, snippetReadBytes)
	n, _ := f.Read(buf)
	f.Close()
	text := cleanSnippet(string(buf[:n]))

	c.mu.Lock()
	c.m[path] = snippetEntry{mtime: mt, text: text}
	c.mu.Unlock()
	return text
}

var (
	snipLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`) // [text](url) → text
	snipInline  = regexp.MustCompile("[*_`#>\\[\\]]")         // residual inline syntax
	snipOrdered = regexp.MustCompile(`^\d+\.\s+`)             // "1. " list marker
	snipWS      = regexp.MustCompile(`\s+`)
)

// cleanSnippet strips markdown chrome from a document's opening and returns the first
// paragraph's prose, collapsed to single spaces and capped at snippetMaxRunes.
func cleanSnippet(raw string) string {
	lines := strings.Split(raw, "\n")
	i := 0
	// Skip a leading YAML frontmatter block.
	if i < len(lines) && strings.TrimSpace(lines[i]) == "---" {
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
			i++
		}
		if i < len(lines) {
			i++
		}
	}
	var prose []string
	for ; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			if len(prose) > 0 {
				break // first paragraph complete
			}
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue // heading
		}
		t = strings.TrimLeft(t, ">-*+ ")
		t = snipOrdered.ReplaceAllString(t, "")
		prose = append(prose, t)
	}
	s := strings.Join(prose, " ")
	s = snipLink.ReplaceAllString(s, "$1")
	s = snipInline.ReplaceAllString(s, "")
	// Drop control bytes and invalid UTF-8 so a stray binary byte can't garble the line.
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || (r < 0x20 && r != '\t') || r == 0x7f {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(snipWS.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > snippetMaxRunes {
		s = strings.TrimSpace(string([]rune(s)[:snippetMaxRunes])) + "…"
	}
	return s
}
