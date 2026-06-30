//go:build !(darwin && cgo && applegrammar)

package main

// appleGrammarChecker returns nil: the default build has no on-device backend and stays
// pure-Go. Build with -tags applegrammar on macOS (cgo) to enable NSSpellChecker + Apple
// Intelligence.
func appleGrammarChecker() grammarChecker { return nil }
