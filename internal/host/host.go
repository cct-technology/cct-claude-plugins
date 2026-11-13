// Package host tells which git host a repository is on, from its `origin`
// remote. qk itself needs only git; what is specific to a host starts here.
package host

import (
	"slices"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
)

// Host is a git host qk knows something about.
type Host struct {
	Name       string   // as people write it
	mark       string   // what the remote's host name contains
	Codeowners []string // where it reads CODEOWNERS, first found first
}

// Known lists the hosts qk knows.
var Known = []Host{
	{"GitHub", "github", []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}},
	{"GitLab", "gitlab", []string{"CODEOWNERS", "docs/CODEOWNERS", ".gitlab/CODEOWNERS"}},
}

// Of returns the host of the remote named origin at root. name is its
// lower-case host name: print that, never the URL, which may hold a password.
// known is what qk knows about the host, nil for one it does not know. ok is
// false when there is no origin.
func Of(root string) (name string, known *Host, ok bool) {
	url, ok := changes.OriginURL(root)
	if !ok {
		return "", nil, false
	}
	name = remoteHost(url)
	if at := slices.IndexFunc(Known, func(h Host) bool { return strings.Contains(name, h.mark) }); at >= 0 {
		known = &Known[at]
	}
	return name, known, true
}

// remoteHost returns the lower-case host name of a git remote URL, in the
// `https://host/path`, `ssh://user@host:port/path` and `user@host:path` forms.
func remoteHost(url string) string {
	if _, rest, ok := strings.Cut(url, "://"); ok {
		url = rest
	}
	host, _, _ := strings.Cut(url, "/")
	host = host[strings.LastIndex(host, "@")+1:] // drop the user, and a password if the URL holds one
	host, _, _ = strings.Cut(host, ":")
	return strings.ToLower(host)
}
