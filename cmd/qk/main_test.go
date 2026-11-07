package main

import (
	"bytes"
	"testing"
)

func TestVersionPrintsQKAndManifestSchemaVersion(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("qk version: %v", err)
	}

	want := "qk dev (manifest schema 1)\n"
	if got := out.String(); got != want {
		t.Errorf("qk version printed %q, want %q", got, want)
	}
}
