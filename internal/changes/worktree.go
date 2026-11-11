package changes

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Worktree is a saved working tree: a copy of every file git reports as
// changed or untracked. Every other file matches the index.
// ponytail: holds those files in memory; ignored files are neither saved nor restored.
type Worktree struct {
	root  string
	files map[string]savedFile // slash path -> content
}

type savedFile struct {
	data   []byte
	mode   fs.FileMode
	exists bool // false: deleted in the working tree
}

// SaveWorktree saves the working tree at root, so Restore can undo what a
// command does to it.
func SaveWorktree(root string) (*Worktree, error) {
	paths, _, err := worktreeStatus(root)
	if err != nil {
		return nil, err
	}
	w := &Worktree{root: root, files: map[string]savedFile{}}
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		info, err := os.Stat(full)
		if errors.Is(err, fs.ErrNotExist) {
			w.files[p] = savedFile{}
			continue
		}
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		w.files[p] = savedFile{data: data, mode: info.Mode().Perm(), exists: true}
	}
	return w, nil
}

// Left is a file Restore left as it is now, although it changed after the tree was saved.
type Left struct {
	Path   string // slash path
	Before string // git blob holding what the file held when saved; "" when git already has that
}

// Restore puts back paths as they were saved: saved files get their content
// back, files that did not exist are removed, and files that matched the index
// are checked out from it. Any other file that changed since the save is left
// alone and returned, since Restore cannot tell who changed it; what it held
// then, when git does not have it, is written to git as a blob.
// ponytail: empty folders a command created stay; git does not see them.
func (w *Worktree) Restore(paths ...string) ([]Left, error) {
	now, untracked, err := worktreeStatus(w.root)
	if err != nil {
		return nil, err
	}
	restore := map[string]bool{}
	for _, p := range paths {
		restore[path.Clean(filepath.ToSlash(p))] = true
	}
	var errs []error
	var fromIndex []string
	var left []Left
	leave := func(p string, s savedFile, saved bool) {
		l := Left{Path: p}
		if saved && s.exists {
			blob, err := keep(w.root, s.data)
			l.Before = blob
			errs = append(errs, err)
		}
		left = append(left, l)
	}
	inStatus := map[string]bool{}
	for _, p := range now {
		inStatus[p] = true
		s, saved := w.files[p]
		switch {
		case restore[p] && saved: // written back below
		case restore[p] && untracked[p]:
			errs = append(errs, removeIfExists(filepath.Join(w.root, filepath.FromSlash(p))))
		case restore[p]:
			fromIndex = append(fromIndex, p)
		case !saved || !s.matches(filepath.Join(w.root, filepath.FromSlash(p))):
			leave(p, s, saved)
		}
	}
	for p, s := range w.files {
		if !inStatus[p] && !restore[p] { // changed before the save, matches the index now
			leave(p, s, true)
		}
	}
	if len(fromIndex) > 0 {
		// Literal: a path like web/app/[id]/page.tsx is not a glob. Saved files are written after, so nothing overwrites them.
		// ponytail: one command line; split it if a generator ever writes thousands of listed files.
		_, err := git(w.root, append([]string{"--literal-pathspecs", "checkout", "--"}, fromIndex...)...)
		errs = append(errs, err)
	}
	for p := range restore {
		if s, saved := w.files[p]; saved {
			errs = append(errs, s.restore(filepath.Join(w.root, filepath.FromSlash(p))))
		}
	}
	slices.SortFunc(left, func(a, b Left) int { return strings.Compare(a.Path, b.Path) })
	return left, errors.Join(errs...)
}

// keep writes data to git as a blob, unchanged, and returns its id.
func keep(root string, data []byte) (string, error) {
	cmd := exec.Command("git", "-C", root, "hash-object", "-w", "--no-filters", "--stdin")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git hash-object: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s savedFile) matches(full string) bool {
	now, err := os.ReadFile(full)
	if errors.Is(err, fs.ErrNotExist) {
		return !s.exists
	}
	return err == nil && s.exists && bytes.Equal(now, s.data)
}

func (s savedFile) restore(full string) error {
	if !s.exists {
		return removeIfExists(full)
	}
	if now, err := os.ReadFile(full); err == nil && bytes.Equal(now, s.data) {
		return os.Chmod(full, s.mode)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(full, s.data, s.mode); err != nil {
		return err
	}
	return os.Chmod(full, s.mode) // WriteFile keeps the mode of a file that exists
}

func removeIfExists(full string) error {
	if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// worktreeStatus lists files that differ from the index or HEAD, and untracked
// files that are not ignored.
func worktreeStatus(root string) (paths []string, untracked map[string]bool, err error) {
	out, err := git(root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--ignore-submodules=all")
	if err != nil {
		return nil, nil, notRepo(err)
	}
	untracked = map[string]bool{}
	for _, rec := range strings.Split(out, "\x00") {
		if len(rec) < 4 {
			continue
		}
		code, p := rec[:2], rec[3:]
		paths = append(paths, p)
		if code == "??" {
			untracked[p] = true
		}
	}
	return paths, untracked, nil
}
