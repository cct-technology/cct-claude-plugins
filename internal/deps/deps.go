// Package deps checks a dependency against its public registry: it exists,
// the version is old enough, it is maintained, and its licence is allowed.
// QK060 uses it for every dependency a change adds, and `qk deps verify` for
// one package. Agents sometimes invent package names, and attackers register
// them; a name that is days old is the sign.
package deps

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// MinAge is how long a version must have been published before it may be
// added. Decided in CCT-3421: 14 days, counted from the publish date of the
// version being added. An urgent patch needs a waiver.
const MinAge = 14 * 24 * time.Hour

// AllowedLicences is the permissive-only list decided in CCT-3421.
var AllowedLicences = []string{"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "0BSD", "Unlicense"}

// Ecosystems are the registries qk can ask, by the name `qk deps verify` takes.
var Ecosystems = []string{"npm", "go", "composer", "cargo", "pypi"}

// Package is one dependency. Version is what the manifest or the person
// named: exact, a range, or empty.
type Package struct {
	Ecosystem string
	Name      string
	Version   string
}

func (p Package) String() string { return fmt.Sprintf("`%s` (%s)", p.Name, p.Ecosystem) }

// Info is what a registry says about one version of a package.
type Info struct {
	Version    string    `json:"version"`   // the version looked at: the one named, or the newest release
	Published  time.Time `json:"published"` // zero when the registry gives no date
	Deprecated string    `json:"deprecated,omitempty"`
	Licence    string    `json:"licence,omitempty"` // an SPDX expression, or what the registry holds
	// NoLicenceData is set for a registry that holds no licences at all (the
	// Go module proxy). An empty Licence without it means none was declared.
	NoLicenceData bool `json:"no_licence_data,omitempty"`
}

// ErrNotFound means the registry answered that the package does not exist.
// Any other error means the registry could not be asked.
var ErrNotFound = errors.New("not found")

// Kinds of problem, so a caller can act on some and not others.
const (
	Missing    = "missing"
	TooYoung   = "too-young"
	Deprecated = "deprecated"
	Licence    = "licence"
)

// Problem is one reason a package should not be added.
type Problem struct {
	Kind    string
	Message string
}

// Verdict is the outcome of checking one package.
type Verdict struct {
	Package  Package
	Info     Info
	Problems []Problem
	Notes    []string // parts that could not be checked
	Err      error    // the registry could not be asked: nothing was checked
}

// Verify checks p against its registry. now is the moment ages are measured from.
func (c *Client) Verify(p Package, now time.Time) Verdict {
	v := Verdict{Package: p}
	info, err := c.Lookup(p)
	if errors.Is(err, ErrNotFound) {
		v.Problems = []Problem{{Missing, fmt.Sprintf("%s does not exist in the registry", p)}}
		return v
	}
	if err != nil {
		v.Err = err
		return v
	}
	v.Info = info
	what := fmt.Sprintf("%s %s", p, info.Version)

	switch age := now.Sub(info.Published); {
	case info.Published.IsZero():
		v.Notes = append(v.Notes, "age not checked: the registry gives no publish date for "+info.Version)
	case age < MinAge:
		v.Problems = append(v.Problems, Problem{TooYoung, fmt.Sprintf(
			"%s was published on %s, %s ago; a new version must be at least %d days old",
			what, info.Published.Format(time.DateOnly), days(age), int(MinAge.Hours()/24))})
	}
	if info.Deprecated != "" {
		v.Problems = append(v.Problems, Problem{Deprecated, fmt.Sprintf("%s is deprecated: %s", what, info.Deprecated)})
	}
	switch {
	case info.NoLicenceData:
		v.Notes = append(v.Notes, "licence not checked: this registry holds no licence data")
	case info.Licence == "":
		v.Problems = append(v.Problems, Problem{Licence, what + " declares no licence"})
	case !Allowed(info.Licence):
		v.Problems = append(v.Problems, Problem{Licence, fmt.Sprintf(
			"%s has the licence `%s`, which is not on the allowed list (%s)", what, info.Licence, strings.Join(AllowedLicences, ", "))})
	}
	return v
}

func days(age time.Duration) string {
	if n := int(age.Hours() / 24); n != 1 {
		return fmt.Sprintf("%d days", max(n, 0))
	}
	return "1 day"
}

// Allowed reports whether an SPDX licence expression can be met with allowed
// licences only: every part of an AND, one part of an OR. `X WITH exception`
// counts as X, because an exception only adds permissions.
func Allowed(expr string) bool {
	// Older manifests write `MIT/Apache-2.0` for a choice.
	expr = strings.NewReplacer("(", " ( ", ")", " ) ", "/", " OR ").Replace(expr)
	p := &licenceParser{tokens: strings.Fields(expr)}
	ok := p.or()
	return ok && p.pos == len(p.tokens)
}

// licenceParser reads SPDX's grammar: AND binds tighter than OR.
type licenceParser struct {
	tokens []string
	pos    int
}

func (p *licenceParser) next(word string) bool {
	if p.pos < len(p.tokens) && strings.EqualFold(p.tokens[p.pos], word) {
		p.pos++
		return true
	}
	return false
}

func (p *licenceParser) or() bool {
	ok := p.and()
	for p.next("OR") {
		ok = p.and() || ok // p.and() first: it must read its tokens either way
	}
	return ok
}

func (p *licenceParser) and() bool {
	ok := p.term()
	for p.next("AND") {
		ok = p.term() && ok
	}
	return ok
}

func (p *licenceParser) term() bool {
	if p.next("(") {
		ok := p.or()
		return p.next(")") && ok
	}
	if p.pos >= len(p.tokens) {
		return false
	}
	id := p.tokens[p.pos]
	p.pos++
	if p.next("WITH") {
		p.pos++ // the exception's name
	}
	return slices.ContainsFunc(AllowedLicences, func(a string) bool { return strings.EqualFold(a, id) })
}
