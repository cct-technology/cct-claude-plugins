package manifest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qualitykit "github.com/cct-technology/cct-claude-plugins"
)

func TestWriteReference(t *testing.T) {
	schema, err := os.ReadFile(filepath.Join("testdata", "reference", "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "reference", "reference.golden"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := WriteReference(&buf, schema); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("reference mismatch\n--- got ---\n%s\n--- want ---\n%s", buf.Bytes(), want)
	}
}

func TestWriteReferenceRejectsWhatItCannotDocument(t *testing.T) {
	tests := []struct {
		name, schema, wantErr string
	}{
		{"unsupported keyword",
			`{"type":"object","properties":{"a":{"description":"A.","oneOf":[{"type":"string"}]}}}`,
			`unsupported schema keyword "oneOf" at a`},
		{"missing description",
			`{"type":"object","properties":{"a":{"type":"string"}}}`,
			"no description for a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteReference(&bytes.Buffer{}, []byte(tt.schema))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// The real schema must stay documentable: every key described, no construct the generator skips.
func TestRealSchemaIsDocumentable(t *testing.T) {
	if err := WriteReference(&bytes.Buffer{}, qualitykit.ManifestSchema); err != nil {
		t.Fatal(err)
	}
}
