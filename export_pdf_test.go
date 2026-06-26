package main

import (
	"bytes"
	"testing"
)

func TestWritePDFManuscriptValid(t *testing.T) {
	doc := ManuscriptDoc{
		{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Plain line."}}}}},
		{Title: "two", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Has "}, {Text: "bold", Bold: true}}}}},
	}
	out, err := writePDF(doc, StyleManuscript, Meta{Author: "Doe", Title: "The Garden"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output is not a PDF (no %%PDF header)")
	}
	if len(out) < 800 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(out))
	}
}

func TestWritePDFSmartQuotesNoError(t *testing.T) {
	// The editor inserts curly quotes/em dashes; the Courier (cp1252) path must transcode.
	doc := ManuscriptDoc{{Title: "q", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "“Curly” — dashes — and an é."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T"})
	if err != nil {
		t.Fatalf("smart-quote text should not error on the Courier path: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestCp1252FallsBackOnUnencodable(t *testing.T) {
	// An astral emoji has no cp1252 encoding -> must fall back, not panic.
	got := cp1252("hi \U0001F600")
	if got == "" {
		t.Fatal("cp1252 returned empty")
	}
}
