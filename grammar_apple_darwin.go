//go:build darwin && cgo && applegrammar

package main

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework FoundationModels -L${SRCDIR} -lokashifm
#include "grammar_apple.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"runtime"
	"strings"
	"unsafe"
)

// appleGrammarChecker picks the best on-device backend: Apple Intelligence when
// available, else the always-present NSSpellChecker.
func appleGrammarChecker() grammarChecker {
	if C.okashi_fm_available() == 1 {
		return fmChecker{}
	}
	return spellChecker{}
}

// --- NSSpellChecker (AppKit) ---

type spellChecker struct{}

func (spellChecker) Name() string    { return "system checker" }
func (spellChecker) Available() bool { return true }

func (spellChecker) Check(text string) ([]grammarFinding, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	var n C.int
	arr := C.okashi_nsspell_check(cs, &n)
	if arr == nil || n == 0 {
		return nil, nil
	}
	defer C.okashi_free_issues(arr, n)
	issues := unsafe.Slice(arr, int(n))
	lines := strings.Split(text, "\n")
	var out []grammarFinding
	for _, is := range issues {
		s := utf16ToRune(text, int(is.location))
		e := utf16ToRune(text, int(is.location+is.length))
		li, sc := runeOffsetToLine(lines, s)
		el, ec := runeOffsetToLine(lines, e)
		if el != li {
			ec = len([]rune(lines[li]))
		}
		if ec < sc {
			ec = sc
		}
		out = append(out, grammarFinding{Line: li, Start: sc, End: ec, Message: C.GoString(is.desc)})
	}
	return out, nil
}

// --- Foundation Models (Apple Intelligence, via the Swift bridge) ---

type fmChecker struct{}

func (fmChecker) Name() string    { return "Apple Intelligence" }
func (fmChecker) Available() bool { return C.okashi_fm_available() == 1 }

func (fmChecker) Check(text string) ([]grammarFinding, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	out := C.okashi_fm_proofread(cs)
	if out == nil {
		return nil, nil
	}
	defer C.free(unsafe.Pointer(out))
	var parsed struct {
		Issues []struct {
			Wrong  string `json:"wrong"`
			Fix    string `json:"fix"`
			Reason string `json:"reason"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(C.GoString(out)), &parsed); err != nil {
		return nil, err
	}
	lines := strings.Split(text, "\n")
	var findings []grammarFinding
	// The model returns the wrong substring verbatim, not an offset. Locate each in
	// document order, claiming successive occurrences so a word repeated on a line maps
	// to distinct spans. LIMITATION: if the model reorders issues, or the same substring
	// occurs before the flagged instance, the underline may land on the wrong occurrence;
	// re-running the check refines it.
	claimedFrom := map[int]int{} // line index → byte offset to resume searching from
	for _, is := range parsed.Issues {
		if is.Wrong == "" {
			continue
		}
		for li, ln := range lines {
			from := claimedFrom[li]
			if from > len(ln) {
				continue
			}
			rel := strings.Index(ln[from:], is.Wrong)
			if rel < 0 {
				continue
			}
			idx := from + rel
			sc := len([]rune(ln[:idx]))
			ec := sc + len([]rune(is.Wrong))
			findings = append(findings, grammarFinding{
				Line: li, Start: sc, End: ec,
				Message:      is.Reason,
				Replacements: []string{is.Fix},
			})
			claimedFrom[li] = idx + len(is.Wrong)
			break
		}
	}
	return findings, nil
}
