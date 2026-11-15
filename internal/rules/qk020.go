package rules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cct-technology/cct-claude-plugins/internal/changes"
	"github.com/cct-technology/cct-claude-plugins/internal/check"
)

// generatorTimeout bounds one generator run, so a hung one cannot hold CI for hours.
// ponytail: a child the generator started may outlive the kill.
const generatorTimeout = 10 * time.Minute

// outputLines is how much of a failing generator's output a finding keeps.
const outputLines = 10

func qk020() check.Rule {
	return check.Rule{
		ID:           "QK020",
		Check:        "generated.current",
		Title:        "Generated files equal their generator's output",
		Phase:        1,
		EnforcedFrom: check.Silver,
		Why: "Reference material is generated from code, never written by hand. For each entry under `generated` in `quality.yaml`, QK020 runs `cmd` from the repository root and compares `file` with what it wrote, ignoring line endings. A command shared by several entries runs once. " +
			"Afterwards the files under `generated` are put back as they were, uncommitted changes included. " +
			"Any other file that changed while the command ran is left as it is, with a warning naming it: qk cannot tell the command's changes from a person's. What such a file held before, if git did not have it, is kept as a git blob the warning names. Ignored files are not restored. " +
			"Generators are slow, so QK020 runs only with `qk check --full` or when the `CI` environment variable is `true`; otherwise it is skipped. " +
			"A generator that fails is a finding with its output. One that is not installed is left out; if nothing else fails, the check is skipped and names it.",
		Fix: "Run the command named in the finding and commit what it writes. Never edit a generated file by hand. For a warning about a file outside `generated`: if the command writes it, list it under `generated`.",
		Run: runQK020,
	}
}

func runQK020(ctx check.Context) check.Result {
	entries := ctx.Manifest.Manifest.Generated
	if len(entries) == 0 {
		return check.Result{Status: check.Pass}
	}
	if !ctx.Full {
		return check.Result{Status: check.Skipped, Reason: "slow: runs with `qk check --full` or in CI"}
	}

	var cmds, declared []string
	files := map[string][]string{}
	for _, e := range entries {
		if _, seen := files[e.Cmd]; !seen {
			cmds = append(cmds, e.Cmd)
		}
		files[e.Cmd] = append(files[e.Cmd], e.File)
		declared = append(declared, e.File)
	}
	var stale, left []check.Finding
	var notRun []string
	for _, cmd := range cmds {
		s, l, err := regenerate(ctx.Root, cmd, files[cmd], declared)
		if errors.Is(err, exec.ErrNotFound) {
			notRun = append(notRun, err.Error()) // one missing tool must not hide what the others find
			continue
		}
		if err != nil {
			return check.Result{Status: check.Skipped, Reason: err.Error()}
		}
		stale, left = append(stale, s...), append(left, l...)
	}
	switch {
	case len(stale) > 0:
		// ponytail: commands that were not installed go unmentioned next to real findings,
		// and files left as they are fail along with the stale ones (SARIF level is per rule).
		return check.Result{Status: check.Fail, Findings: append(stale, left...)}
	case len(left) > 0:
		return check.Result{Status: check.Warn, Findings: left}
	case len(notRun) > 0:
		return check.Result{Status: check.Skipped, Reason: strings.Join(notRun, "; ")}
	}
	return check.Result{Status: check.Pass}
}

// regenerate runs cmd in root, reports each of files that then differs, and
// puts back every declared file. Other files that changed meanwhile are left
// as they are and reported, since cmd and a person editing are indistinguishable.
// An error means cmd could not be checked: it is not installed
// (exec.ErrNotFound), or git could not save or restore the tree.
func regenerate(root, cmd string, files, declared []string) (stale, left []check.Finding, err error) {
	tree, err := changes.SaveWorktree(root)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot save the working tree before running `%s`: %w", cmd, err)
	}
	before := map[string][]byte{}
	for _, f := range files {
		before[f] = readFile(root, f)
	}
	out, runErr := runGenerator(root, cmd)
	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, nil, fmt.Errorf("`%s` is not installed: %w", cmd, runErr)
	}
	for _, f := range files {
		after := readFile(root, f)
		switch {
		case runErr != nil:
			stale = append(stale, check.Finding{File: f, Message: fmt.Sprintf("`%s` failed (%v):\n%s", cmd, runErr, tail(out))})
		case before[f] == nil && after == nil:
			stale = append(stale, check.Finding{File: f, Message: fmt.Sprintf("does not exist, and `%s` does not write it", cmd)})
		case !sameText(before[f], after):
			stale = append(stale, check.Finding{File: f, Message: fmt.Sprintf("differs from what `%s` writes", cmd)})
		}
	}

	others, err := tree.Restore(declared...)
	if err != nil {
		return nil, nil, fmt.Errorf("could not put back what `%s` wrote; see git status: %w", cmd, err)
	}
	for _, o := range others {
		msg := fmt.Sprintf("changed while `%s` ran, and is not under `generated`. qk cannot tell whether the command or a person changed it, so it left the file as it is now. If the command writes it, list it under `generated`.", cmd)
		if o.Before != "" {
			msg += fmt.Sprintf(" What it held before: `git cat-file -p %s`.", o.Before)
		}
		left = append(left, check.Finding{File: o.Path, Message: msg})
	}
	return stale, left, nil
}

// runGenerator runs cmd without a shell: it is split on spaces, with no quoting.
func runGenerator(root, cmd string) ([]byte, error) {
	args := strings.Fields(cmd)
	if len(args) == 0 {
		return nil, errors.New("no command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), generatorTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = root
	c.WaitDelay = 5 * time.Second
	out, err := c.CombinedOutput()
	if ctx.Err() != nil {
		err = fmt.Errorf("did not finish within %v", generatorTimeout)
	}
	return out, err
}

// readFile returns nil for a file that does not exist.
func readFile(root, file string) []byte {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return nil
	}
	if data == nil {
		data = []byte{} // exists, empty
	}
	return data
}

func sameText(a, b []byte) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	crlf, lf := []byte("\r\n"), []byte("\n")
	return bytes.Equal(bytes.ReplaceAll(a, crlf, lf), bytes.ReplaceAll(b, crlf, lf))
}

func tail(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > outputLines {
		lines = append([]string{"..."}, lines[len(lines)-outputLines:]...)
	}
	return strings.Join(lines, "\n")
}
