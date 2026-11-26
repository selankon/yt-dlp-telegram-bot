package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunUpdateCmd(t *testing.T) {
	out, err := runUpdateCmd(context.Background(), "echo hello; echo oops >&2")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello\noops" {
		t.Fatalf("output = %q", out)
	}

	out, err = runUpdateCmd(context.Background(), "echo failing; exit 2")
	if err == nil || out != "failing" {
		t.Fatalf("expected error with output, got %q %v", out, err)
	}

	out, _ = runUpdateCmd(context.Background(), "head -c 5000 /dev/zero | tr '\\0' x")
	if r := []rune(out); len(r) != maxUpdateOutput+1 || !strings.HasPrefix(out, "…") {
		t.Fatalf("output not truncated: %d runes", len(r))
	}
}
