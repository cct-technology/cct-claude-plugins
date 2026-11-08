package glob

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"a.txt", "a.txt", true},
		{"a.txt", "b.txt", false},
		{"a.txt", "dir/a.txt", false},

		// * stays inside one path segment.
		{"*.go", "main.go", true},
		{"*.go", "cmd/main.go", false},
		{"internal/*/x.go", "internal/pkg/x.go", true},
		{"internal/*/x.go", "internal/a/b/x.go", false},
		{"cmd/*", "cmd/qk", true},

		// ? is exactly one character.
		{"v?.txt", "v1.txt", true},
		{"v?.txt", "v12.txt", false},

		// ** is zero or more whole segments.
		{"**", "a/b/c.txt", true},
		{"internal/config/**", "internal/config/load.go", true},
		{"internal/config/**", "internal/config/sub/load.go", true},
		{"internal/config/**", "internal/config", true},
		{"internal/config/**", "internal/configs/load.go", false},
		{"**/x.go", "x.go", true},
		{"**/x.go", "a/b/x.go", true},
		{"a/**/z.txt", "a/z.txt", true},
		{"a/**/z.txt", "a/b/c/z.txt", true},
		{"a/**/z.txt", "a/b/c/y.txt", false},

		// Parentheses and brackets are literal: Next.js route folders.
		{"web/app/(admin)/**", "web/app/(admin)/users/page.tsx", true},
		{"web/app/(admin)/**", "web/app/admin/users/page.tsx", false},
		{"app/[id]/page.tsx", "app/[id]/page.tsx", true},
		{"app/[id]/page.tsx", "app/i/page.tsx", false},
		{"app/[id]/*.tsx", "app/[id]/page.tsx", true},

		{"docs/how-to/*notes.md", "docs/how-to/kvalvågnes notes.md", true},
		{"docs/how-to/?.md", "docs/how-to/å.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" on "+tt.name, func(t *testing.T) {
			if got := Match(tt.pattern, tt.name); got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}
