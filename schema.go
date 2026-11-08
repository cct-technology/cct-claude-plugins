// Package qualitykit embeds plugin files that qk needs at runtime.
// It sits at the repo root because go:embed cannot reach outside a package's directory.
package qualitykit

import _ "embed"

// ManifestSchema is plugins/quality-kit/schemas/quality.schema.json: the one
// source for validating quality.yaml and for its generated reference.
//
//go:embed plugins/quality-kit/schemas/quality.schema.json
var ManifestSchema []byte
