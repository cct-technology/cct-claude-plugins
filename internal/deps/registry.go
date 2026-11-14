package deps

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// cacheFor is how long a registry answer is reused. Only the facts are kept
// (the version, its date, its licence), never a verdict, so a version that
// was too young yesterday is judged again today.
const cacheFor = 24 * time.Hour

// Client asks the public registries. The URLs are fields so tests can point
// them at a fake registry.
// ponytail: public registries only. A package from a private registry reads
// as missing; honour .npmrc, GOPRIVATE and composer repositories when a team
// with private packages needs it.
type Client struct {
	NPM, GoProxy, Packagist, Crates, PyPI string
	CacheDir                              string // "" for no cache
	HTTP                                  *http.Client
	Now                                   func() time.Time
}

// Default is the client for the real registries, caching in the user's cache folder.
func Default() *Client {
	c := &Client{
		NPM:       "https://registry.npmjs.org",
		GoProxy:   "https://proxy.golang.org",
		Packagist: "https://packagist.org",
		Crates:    "https://crates.io",
		PyPI:      "https://pypi.org",
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		Now:       time.Now,
	}
	// Not the plugin's data folder: qk also runs before a push and in CI.
	if dir, err := os.UserCacheDir(); err == nil {
		c.CacheDir = filepath.Join(dir, "quality-kit", "registry")
	}
	return c
}

// Lookup returns what the registry says about p: about the version p names
// when the registry has exactly that version, otherwise about the newest
// release. ErrNotFound means there is no such package; any other error means
// the registry could not be asked.
func (c *Client) Lookup(p Package) (Info, error) {
	want := namedVersion(p.Version)
	if info, ok := c.cached(p.Ecosystem, p.Name, want); ok {
		return info, nil
	}
	var info Info
	var err error
	switch p.Ecosystem {
	case "npm":
		info, err = c.npm(p.Name, want)
	case "go":
		info, err = c.goModule(p.Name, want)
	case "composer":
		info, err = c.packagist(p.Name, want)
	case "cargo":
		info, err = c.crate(p.Name, want)
	case "pypi":
		info, err = c.pypi(p.Name, want)
	default:
		return Info{}, fmt.Errorf("unknown ecosystem %q (want one of %s)", p.Ecosystem, strings.Join(Ecosystems, ", "))
	}
	if err != nil {
		return Info{}, err
	}
	c.store(p.Ecosystem, p.Name, want, info)
	return info, nil
}

var versionInSpec = regexp.MustCompile(`[0-9][0-9A-Za-z.+_-]*`)

// namedVersion is the first version a spec such as `^1.2.3`, `>=2.0,<3` or
// `v1.4.0` names, or "" when it names none (`*`, `latest`).
// ponytail: the version the manifest names, not the one the lockfile
// resolved. `npm install x` writes `^<newest>`, so for a dependency that was
// just added they agree; read the lockfiles when they do not.
func namedVersion(spec string) string {
	v := versionInSpec.FindString(spec)
	if strings.HasPrefix(strings.TrimSpace(spec), "v") && v != "" {
		return "v" + v // Go versions keep their v
	}
	return v
}

// get fetches a URL. A 404 or 410 is ErrNotFound; anything else that is not a
// 200 is an error, which callers treat as "could not ask".
func (c *Client) get(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// crates.io refuses requests without one. It names the tool only.
	req.Header.Set("User-Agent", "quality-kit-qk (dependency check)")
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		// An npm document for a package with thousands of versions runs to tens of megabytes.
		return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	case http.StatusNotFound, http.StatusGone:
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
}

func (c *Client) getJSON(rawURL string, into any) error {
	body, err := c.get(rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("%s: unreadable answer: %w", rawURL, err)
	}
	return nil
}

func (c *Client) npm(name, want string) (Info, error) {
	var doc struct {
		DistTags map[string]string `json:"dist-tags"`
		// Not all of these are dates: an unpublished package has an object here.
		Time     map[string]json.RawMessage `json:"time"`
		Versions map[string]struct {
			Deprecated json.RawMessage `json:"deprecated"`
			License    json.RawMessage `json:"license"`
			Licenses   json.RawMessage `json:"licenses"`
		} `json:"versions"`
	}
	// A scoped name keeps its @ and has its slash escaped.
	if err := c.getJSON(c.NPM+"/"+strings.ReplaceAll(name, "/", "%2F"), &doc); err != nil {
		return Info{}, err
	}
	v, ok := doc.Versions[want]
	if !ok {
		want = doc.DistTags["latest"]
		if v, ok = doc.Versions[want]; !ok {
			return Info{}, ErrNotFound // unpublished: the name is there, no version is
		}
	}
	info := Info{Version: want}
	_ = json.Unmarshal(doc.Time[want], &info.Published)
	var deprecated string
	if json.Unmarshal(v.Deprecated, &deprecated) == nil {
		info.Deprecated = deprecated
	}
	info.Licence = npmLicence(v.License)
	if info.Licence == "" {
		info.Licence = npmLicence(v.Licenses)
	}
	return info, nil
}

// npmLicence reads the three shapes npm has allowed: "MIT", {"type": "MIT"},
// and [{"type": "MIT"}, ...] for a choice.
func npmLicence(raw json.RawMessage) string {
	type typed struct {
		Type string `json:"type"`
	}
	var s string
	var one typed
	var many []typed
	switch {
	case json.Unmarshal(raw, &s) == nil:
		return s
	case json.Unmarshal(raw, &one) == nil:
		return one.Type
	case json.Unmarshal(raw, &many) == nil:
		var ids []string
		for _, l := range many {
			ids = append(ids, l.Type)
		}
		return strings.Join(ids, " OR ")
	}
	return ""
}

func (c *Client) goModule(name, want string) (Info, error) {
	// The proxy writes an upper-case letter as ! and its lower case.
	var escaped strings.Builder
	for _, r := range name {
		if unicode.IsUpper(r) {
			escaped.WriteByte('!')
			r = unicode.ToLower(r)
		}
		escaped.WriteRune(r)
	}
	base := c.GoProxy + "/" + escaped.String()

	var latest, at struct {
		Version string
		Time    time.Time
	}
	if err := c.getJSON(base+"/@latest", &latest); err != nil {
		return Info{}, err
	}
	at = latest
	if want != "" && want != latest.Version {
		if err := c.getJSON(base+"/@v/"+want+".info", &at); err == ErrNotFound {
			at = latest
		} else if err != nil {
			return Info{}, err
		}
	}
	info := Info{Version: at.Version, Published: at.Time, NoLicenceData: true}
	// A module is deprecated by a comment in the go.mod of its newest version.
	mod, err := c.get(base + "/@v/" + latest.Version + ".mod")
	if err != nil && err != ErrNotFound {
		return Info{}, err
	}
	for _, line := range strings.Split(string(mod), "\n") {
		if why, ok := strings.CutPrefix(strings.TrimSpace(line), "// Deprecated:"); ok {
			info.Deprecated = strings.TrimSpace(why)
			break
		}
	}
	return info, nil
}

func (c *Client) packagist(name, want string) (Info, error) {
	var doc struct {
		Package struct {
			Abandoned json.RawMessage `json:"abandoned"`
			Versions  map[string]struct {
				Time    time.Time `json:"time"`
				License []string  `json:"license"`
			} `json:"versions"`
		} `json:"package"`
	}
	if err := c.getJSON(c.Packagist+"/packages/"+name+".json", &doc); err != nil {
		return Info{}, err
	}
	versions := doc.Package.Versions
	v, ok := versions[want]
	if !ok {
		v, ok = versions["v"+want]
		want = "v" + want
	}
	if !ok {
		want = ""
		for num, candidate := range versions { // the newest release that is not a branch
			if !strings.Contains(num, "dev") && (want == "" || candidate.Time.After(v.Time)) {
				want, v = num, candidate
			}
		}
		if want == "" {
			return Info{}, ErrNotFound
		}
	}
	info := Info{Version: want, Published: v.Time, Licence: strings.Join(v.License, " OR ")}
	var replacement string
	switch abandoned := string(doc.Package.Abandoned); {
	case json.Unmarshal(doc.Package.Abandoned, &replacement) == nil && replacement != "":
		info.Deprecated = "abandoned; use " + replacement
	case abandoned == "true":
		info.Deprecated = "abandoned"
	}
	return info, nil
}

func (c *Client) crate(name, want string) (Info, error) {
	var doc struct {
		Crate struct {
			MaxStable string `json:"max_stable_version"`
			Max       string `json:"max_version"`
		} `json:"crate"`
		Versions []struct {
			Num     string    `json:"num"`
			Created time.Time `json:"created_at"`
			License string    `json:"license"`
			Yanked  bool      `json:"yanked"`
		} `json:"versions"`
	}
	if err := c.getJSON(c.Crates+"/api/v1/crates/"+url.PathEscape(name), &doc); err != nil {
		return Info{}, err
	}
	newest := doc.Crate.MaxStable
	if newest == "" {
		newest = doc.Crate.Max
	}
	for _, num := range []string{want, newest} {
		for _, v := range doc.Versions {
			if v.Num != num || num == "" {
				continue
			}
			info := Info{Version: v.Num, Published: v.Created, Licence: v.License}
			if v.Yanked {
				info.Deprecated = "this version was yanked"
			}
			return info, nil
		}
	}
	return Info{}, ErrNotFound
}

func (c *Client) pypi(name, want string) (Info, error) {
	var doc struct {
		Info struct {
			Version           string   `json:"version"`
			License           string   `json:"license"`
			LicenseExpression string   `json:"license_expression"`
			Classifiers       []string `json:"classifiers"`
		} `json:"info"`
		Releases map[string][]struct {
			Uploaded     time.Time `json:"upload_time_iso_8601"`
			Yanked       bool      `json:"yanked"`
			YankedReason string    `json:"yanked_reason"`
		} `json:"releases"`
	}
	if err := c.getJSON(c.PyPI+"/pypi/"+url.PathEscape(name)+"/json", &doc); err != nil {
		return Info{}, err
	}
	files, ok := doc.Releases[want]
	if !ok || len(files) == 0 {
		want = doc.Info.Version
		if files = doc.Releases[want]; len(files) == 0 {
			return Info{}, ErrNotFound // a name with no files to install
		}
	}
	// ponytail: the licence is the newest release's; PyPI gives one per project here.
	info := Info{Version: want, Published: files[0].Uploaded, Licence: pypiLicence(doc.Info.LicenseExpression, doc.Info.License, doc.Info.Classifiers)}
	yanked := true
	for _, f := range files {
		if f.Uploaded.Before(info.Published) {
			info.Published = f.Uploaded
		}
		yanked = yanked && f.Yanked
	}
	switch {
	case yanked:
		info.Deprecated = strings.TrimSpace("this version was yanked " + files[0].YankedReason)
	case slices.Contains(doc.Info.Classifiers, "Development Status :: 7 - Inactive"):
		info.Deprecated = "the project is marked inactive"
	}
	return info, nil
}

// pypiNames maps what PyPI projects write, in classifiers and in the old
// free-text field, to SPDX ids.
// ponytail: the common spellings of the allowed licences only. Anything else
// is reported as written, and a person reads it.
var pypiNames = map[string]string{
	"mit license": "MIT", "the mit license": "MIT", "mit": "MIT",
	"apache software license": "Apache-2.0", "apache 2.0": "Apache-2.0", "apache-2.0": "Apache-2.0",
	"apache license 2.0": "Apache-2.0", "apache license, version 2.0": "Apache-2.0",
	"isc license (iscl)": "ISC", "isc license": "ISC", "isc": "ISC",
	"the unlicense (unlicense)": "Unlicense", "unlicense": "Unlicense",
	"zero-clause bsd (0bsd)": "0BSD", "0bsd": "0BSD",
	"bsd-3-clause": "BSD-3-Clause", "bsd 3-clause": "BSD-3-Clause", "3-clause bsd license": "BSD-3-Clause", "new bsd license": "BSD-3-Clause",
	"bsd-2-clause": "BSD-2-Clause", "bsd 2-clause": "BSD-2-Clause", "2-clause bsd license": "BSD-2-Clause", "simplified bsd license": "BSD-2-Clause",
}

// pypiLicence picks the best statement of a project's licence: the SPDX
// expression, then the free-text field when it is a name and not the whole
// licence text, then the classifiers.
func pypiLicence(expression, text string, classifiers []string) string {
	if expression != "" {
		return expression
	}
	spdx := func(name string) string {
		if id, ok := pypiNames[strings.ToLower(strings.TrimSpace(name))]; ok {
			return id
		}
		return strings.TrimSpace(name)
	}
	if text = strings.TrimSpace(text); text != "" && len(text) <= 64 && !strings.Contains(text, "\n") {
		return spdx(text)
	}
	var ids []string
	for _, c := range classifiers {
		if name, ok := strings.CutPrefix(c, "License :: "); ok {
			ids = append(ids, spdx(strings.TrimPrefix(name, "OSI Approved :: ")))
		}
	}
	return strings.Join(ids, " OR ")
}

// cacheFile names the file that holds the answer for one package and version.
func (c *Client) cacheFile(ecosystem, name, version string) string {
	sum := sha256.Sum256([]byte(ecosystem + "\x00" + name + "\x00" + version))
	return filepath.Join(c.CacheDir, fmt.Sprintf("%x.json", sum[:16]))
}

type cacheEntry struct {
	Fetched time.Time `json:"fetched"`
	Info    Info      `json:"info"`
}

func (c *Client) cached(ecosystem, name, version string) (Info, bool) {
	if c.CacheDir == "" {
		return Info{}, false
	}
	data, err := os.ReadFile(c.cacheFile(ecosystem, name, version))
	var e cacheEntry
	if err != nil || json.Unmarshal(data, &e) != nil || c.Now().Sub(e.Fetched) > cacheFor {
		return Info{}, false
	}
	return e.Info, true
}

// store keeps an answer. A package that was not found is never kept: it may
// be published a minute later. A cache that cannot be written is not an error.
func (c *Client) store(ecosystem, name, version string, info Info) {
	if c.CacheDir == "" {
		return
	}
	data, err := json.Marshal(cacheEntry{Fetched: c.Now(), Info: info})
	if err != nil || os.MkdirAll(c.CacheDir, 0o755) != nil {
		return
	}
	_ = os.WriteFile(c.cacheFile(ecosystem, name, version), data, 0o644)
}
