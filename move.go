package main

import (
	"errors"
	"os"
	"syscall"
)

// safeMove moves a file from src to dst. It tries os.Rename (fast, same-volume) and, only on a
// cross-device error, falls back to copy-then-remove. dst's parent directory must already exist.
func safeMove(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	// Cross-volume: copy the bytes atomically to dst, then remove the source.
	data, rerr := os.ReadFile(src)
	if rerr != nil {
		return rerr
	}
	if werr := atomicWrite(dst, data, 0o644); werr != nil {
		return werr
	}
	return os.Remove(src)
}
