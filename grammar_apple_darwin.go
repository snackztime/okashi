//go:build darwin && cgo && applegrammar

package main

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework FoundationModels -L${SRCDIR} -lokashifm
#include "grammar_apple.h"
#include <stdlib.h>
*/
import "C"

import (
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
	// Parse + locate is pure-Go (unit-tested in grammar_backend_test.go).
	return fmFindings(C.GoString(out), text)
}
