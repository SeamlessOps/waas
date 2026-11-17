// Package actions writes what a GitHub Actions step reports: its outputs and
// its summary. Outside Actions (no GITHUB_OUTPUT or GITHUB_STEP_SUMMARY) both
// are no-ops, so the CLI runs the same on a laptop.
package actions

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Output sets the step outputs in values, in order. Each is written as a
// delimited block, so a value may hold any text, newlines included.
func Output(values ...[2]string) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	var b strings.Builder
	for _, kv := range values {
		delim := delimiter()
		fmt.Fprintf(&b, "%s<<%s\n%s\n%s\n", kv[0], delim, kv[1], delim)
	}
	return appendTo(path, b.String())
}

// Summary adds markdown to the step summary.
func Summary(markdown string) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	return appendTo(path, markdown+"\n")
}

func appendTo(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func delimiter() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "WAAS_" + hex.EncodeToString(b)
}
