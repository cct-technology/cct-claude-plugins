// Package changes answers "what changed on this branch?" for every rule:
// the files changed between the base and the working tree, and the commit
// trailers in between. It shells out to git.
package changes

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Status is how a file changed.
type Status string

const (
	Added    Status = "added"
	Modified Status = "modified"
	Deleted  Status = "deleted"
	Renamed  Status = "renamed"
)

// File is one changed file. Paths are slash-separated and relative to the repo root.
type File struct {
	Path    string
	OldPath string // the path before a rename
	Status  Status
}

// Trailer is one "Key: value" trailer of a commit in base..HEAD.
type Trailer struct {
	Commit string
	Key    string
	Value  string
}

// Set is what changed between the base and the working tree.
type Set struct {
	Root     string
	Base     string // commit hash
	Files    []File
	Trailers []Trailer
}

// Reasons a change set cannot be computed. Rules that need one skip with the error as reason.
var (
	ErrNotRepo         = errors.New("not a git repository")
	ErrNoCommits       = errors.New("the repository has no commits yet")
	ErrNoDefaultBranch = errors.New("cannot find the default branch (no origin/HEAD, main or master); pass --base")
	ErrBadBase         = errors.New("--base does not name a commit")
)

// Root returns the top level of the git repository containing dir.
func Root(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", notRepo(err)
	}
	return filepath.FromSlash(strings.TrimSpace(out)), nil
}

// Compute returns the change set for the repository at root. base is a ref
// to compare against; empty means the default branch. Either way the base is
// the merge-base with HEAD, as in a pull request.
func Compute(root, base string) (*Set, error) {
	if _, err := Root(root); err != nil {
		return nil, err
	}
	if !verify(root, "HEAD") {
		return nil, ErrNoCommits
	}
	ref := base
	if ref == "" {
		var err error
		if ref, err = defaultBranch(root); err != nil {
			return nil, err
		}
	} else if !verify(root, ref) {
		return nil, fmt.Errorf("%w: %q", ErrBadBase, base)
	}
	mb, err := git(root, "merge-base", ref, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("merge-base of %s and HEAD: %w", ref, err)
	}
	s := &Set{Root: root, Base: strings.TrimSpace(mb)}
	if s.Files, err = changedFiles(root, s.Base); err != nil {
		return nil, err
	}
	if s.Trailers, err = trailers(root, s.Base); err != nil {
		return nil, err
	}
	return s, nil
}

// ContentAtBase returns path's content at the base; ok is false when it did not exist there.
func (s *Set) ContentAtBase(path string) (content []byte, ok bool, err error) {
	obj := s.Base + ":" + path
	if !succeeds(s.Root, "cat-file", "-e", obj) {
		return nil, false, nil
	}
	out, err := git(s.Root, "cat-file", "blob", obj)
	return []byte(out), err == nil, err
}

// defaultBranch prefers origin/HEAD, so commits not yet pushed count as changes.
func defaultBranch(root string) (string, error) {
	if out, err := git(root, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimSpace(out), nil
	}
	for _, b := range []string{"main", "master"} {
		if verify(root, "refs/heads/"+b) {
			return b, nil
		}
	}
	return "", ErrNoDefaultBranch
}

// changedFiles lists tracked changes from base to the working tree (staged or
// not), then untracked files that are not ignored.
func changedFiles(root, base string) ([]File, error) {
	out, err := git(root, "diff", "--name-status", "-z", "-M", "--no-color", "--no-ext-diff", base)
	if err != nil {
		return nil, err
	}
	var files []File
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		code, path := fields[i], fields[i+1]
		f := File{Path: path}
		switch code[0] {
		case 'A':
			f.Status = Added
		case 'D':
			f.Status = Deleted
		case 'R':
			if i+2 >= len(fields) {
				return nil, fmt.Errorf("git diff: rename without a new path: %q", out)
			}
			f = File{OldPath: path, Path: fields[i+2], Status: Renamed}
			i++
		default: // M, T (type change), C, U
			f.Status = Modified
		}
		files = append(files, f)
	}

	out, err = git(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			files = append(files, File{Path: p, Status: Added})
		}
	}
	return files, nil
}

// trailers parses the trailers of every commit in base..HEAD, oldest first.
func trailers(root, base string) ([]Trailer, error) {
	out, err := git(root, "-c", "log.showSignature=false", "log", "--reverse", "--no-color", "-z",
		"--format=%H%n%(trailers:only,unfold)", base+"..HEAD")
	if err != nil {
		return nil, err
	}
	var ts []Trailer
	for _, entry := range strings.Split(out, "\x00") {
		lines := strings.Split(strings.TrimSpace(entry), "\n")
		if lines[0] == "" {
			continue
		}
		for _, line := range lines[1:] {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			ts = append(ts, Trailer{Commit: lines[0], Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)})
		}
	}
	return ts, nil
}

// verify reports whether ref names a commit, by exit code only (git's messages may be localised).
func verify(root, ref string) bool {
	return succeeds(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

func succeeds(root string, args ...string) bool {
	_, err := git(root, args...)
	return err == nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// notRepo maps git failing to start, or failing in a folder outside any repo, to ErrNotRepo.
func notRepo(err error) error {
	var exitErr *exec.ExitError
	if errors.Is(err, exec.ErrNotFound) || !errors.As(err, &exitErr) {
		return fmt.Errorf("%w: git is not available: %v", ErrNotRepo, err)
	}
	return fmt.Errorf("%w: %v", ErrNotRepo, err)
}
