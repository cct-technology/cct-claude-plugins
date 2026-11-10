package changes

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// Restore puts the working tree back as it was saved. Saved files get their
// content back, files that did not exist are removed, new untracked files are
// deleted, and tracked files that were unchanged are checked out from the index.
// ponytail: empty folders a command created stay; git does not see them.
func (w *Worktree) Restore() error {
	now, untracked, err := worktreeStatus(w.root)
	if err != nil {
		return err
	}
	var errs []error
	var fromIndex []string
	for _, p := range now {
		switch _, saved := w.files[p]; {
		case saved:
		case untracked[p]:
			errs = append(errs, removeIfExists(filepath.Join(w.root, filepath.FromSlash(p))))
		default:
			fromIndex = append(fromIndex, p)
		}
	}
	if len(fromIndex) > 0 {
		// Literal: a path like web/app/[id]/page.tsx is not a glob. Saved files are written after, so nothing overwrites them.
		// ponytail: one command line; split it if a generator ever touches thousands of files.
		_, err := git(w.root, append([]string{"--literal-pathspecs", "checkout", "--"}, fromIndex...)...)
		errs = append(errs, err)
	}
	for p, s := range w.files {
		errs = append(errs, s.restore(filepath.Join(w.root, filepath.FromSlash(p))))
	}
	return errors.Join(errs...)
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
