// Package glob matches slash-separated, repo-relative paths: * within one
// segment, ? for one character, ** for zero or more whole segments. Everything
// else is literal, including the brackets and parentheses of Next.js routes
// such as app/[id]/page.tsx and app/(admin)/.
package glob

import "strings"

// Match reports whether name matches pattern.
func Match(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pats, segs []string) bool {
	if len(pats) == 0 {
		return len(segs) == 0
	}
	if pats[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchSegments(pats[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	return len(segs) > 0 && matchSegment([]rune(pats[0]), []rune(segs[0])) && matchSegments(pats[1:], segs[1:])
}

func matchSegment(pat, s []rune) bool {
	if len(pat) == 0 {
		return len(s) == 0
	}
	switch pat[0] {
	case '*':
		for i := 0; i <= len(s); i++ {
			if matchSegment(pat[1:], s[i:]) {
				return true
			}
		}
		return false
	case '?':
		return len(s) > 0 && matchSegment(pat[1:], s[1:])
	}
	return len(s) > 0 && pat[0] == s[0] && matchSegment(pat[1:], s[1:])
}
