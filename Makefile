# okashi build targets.
GO := /opt/homebrew/bin/go
SWIFT_LIB := $(shell xcrun --show-sdk-path)/usr/lib/swift

# Default: pure-Go, cross-platform, no cgo, no Apple backend.
.PHONY: build
build:
	$(GO) build -o okashi .

# Apple build: NSSpellChecker + Foundation Models. Requires macOS + Xcode (cgo + swiftc).
# Compiles the Swift FM bridge to a static lib, then builds with the applegrammar tag.
.PHONY: apple
apple: libokashifm.a
	CGO_ENABLED=1 CGO_LDFLAGS="-L$(SWIFT_LIB)" $(GO) build -tags applegrammar -o okashi-apple .

libokashifm.a: grammar_apple_fm.swift
	xcrun swiftc -emit-library -static -o libokashifm.a grammar_apple_fm.swift -framework FoundationModels

.PHONY: clean
clean:
	rm -f okashi okashi-apple libokashifm.a
