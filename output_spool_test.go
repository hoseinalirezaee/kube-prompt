package main

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestOutputSpoolAppendsAndReadsLines(t *testing.T) {
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()

	if err := spool.Append([]byte("first\nsec")); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := spool.Append([]byte("ond\nthird")); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	if got, want := spool.LineCount(), 3; got != want {
		t.Fatalf("expected %d lines, got %d", want, got)
	}
	lines, err := spool.ReadLines(0, 10)
	if err != nil {
		t.Fatalf("read lines failed: %v", err)
	}
	want := []string{"first", "second", "third"}
	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %#v", len(want), len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d: expected %q, got %q", i, want[i], lines[i])
		}
	}
}

func TestOutputSpoolTrailingNewlineDoesNotAddEmptyLine(t *testing.T) {
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()

	if err := spool.Append([]byte("first\nsecond\n")); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if got, want := spool.LineCount(), 2; got != want {
		t.Fatalf("expected %d lines, got %d", want, got)
	}
	hasOutput, endedWithNewline := spool.OutputState()
	if !hasOutput || !endedWithNewline {
		t.Fatalf("expected output ending with newline, got hasOutput=%t endedWithNewline=%t", hasOutput, endedWithNewline)
	}
}

func TestOutputSpoolWriteTo(t *testing.T) {
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()

	if _, err := spool.Write([]byte("first\nsecond\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	var out bytes.Buffer
	if err := spool.WriteTo(&out); err != nil {
		t.Fatalf("write to failed: %v", err)
	}
	if got, want := out.String(), "first\nsecond\n"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestOutputSpoolOutputStateWithoutTrailingNewline(t *testing.T) {
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	defer spool.Close()

	if err := spool.Append([]byte("first")); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	hasOutput, endedWithNewline := spool.OutputState()
	if !hasOutput || endedWithNewline {
		t.Fatalf("expected output without trailing newline, got hasOutput=%t endedWithNewline=%t", hasOutput, endedWithNewline)
	}
}

func TestOutputSpoolCloseRemovesTempFile(t *testing.T) {
	spool, err := newOutputSpool()
	if err != nil {
		t.Fatalf("expected spool, got error %v", err)
	}
	path := spool.path
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected temp file to exist: %v", err)
	}

	if err := spool.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp file to be removed, stat error %v", err)
	}
}
