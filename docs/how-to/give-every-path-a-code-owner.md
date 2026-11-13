# Give every path a code owner

QK110 checks that each file and folder at the top of the repository has an owner in `CODEOWNERS`. Every finding needs someone to act on it, and every change someone to review it.

QK110 needs no configuration.

## Where the file goes

QK110 reads `CODEOWNERS` where your git host reads it. It takes the host from the `origin` remote.

| Host | Locations, first found first |
| --- | --- |
| GitHub | `.github/CODEOWNERS`, `CODEOWNERS`, `docs/CODEOWNERS` |
| GitLab | `CODEOWNERS`, `docs/CODEOWNERS`, `.gitlab/CODEOWNERS` |

A host counts as GitHub or GitLab when its name contains `github` or `gitlab`, so `github.example.com` works too.

On another host, or without an `origin` remote, QK110 is skipped and says so. To stop the notice, set `ownership.codeowners: "off"` under `modes` in `quality.yaml`.

At `gold`, QK110 blocks. At `bronze` and `silver`, and without a `quality.yaml`, it warns. To make it block at `silver`, set `ownership.codeowners: enforced` under `modes`.

## Write the file

The shortest file that passes gives everything a default owner:

```text
* @your-team
```

Add lines below it for paths with their own owner. The last line that matches wins:

```text
*            @your-team
/docs/       @your-team/writers
/api/        @ann ben@example.com
```

An owner is a `@user`, a `@org/team` or an email address.

## What counts as owned

- **A top-level file** is owned when the last line that matches it names an owner.
- **A top-level folder** is owned when a new file in it would get an owner. `/docs/ @ann` owns `docs/`. `*.md @ann` owns the Markdown files in `docs/`, not `docs/` itself.

QK110 looks only at the top level. A line that leaves a subfolder without an owner, such as `/apps/github` below `/apps/ @ann`, passes.

## Read the result

```text
FAIL  QK110  Every top-level path has a code owner (enforced)
      - LICENSE: no owner: no line in `.github/CODEOWNERS` matches this file
      - docs: no owner: line 7 of `.github/CODEOWNERS` (`/docs/`) names none
      - scripts: no owner: no line in `.github/CODEOWNERS` covers this folder
```

- **No line matches or covers the path.** Add a line for it, or a default owner on the first line.
- **A line names none.** A line with a pattern and no owner takes ownership away from what it matches. Give it an owner, or remove it.
- **No CODEOWNERS file.** Create it in one of the locations for your host.

If a path should have no owner, ask a person for a [waiver](waive-a-finding.md):

```sh
qk waiver add QK110 --path scripts --reason "Throwaway scripts, removed in January" --owner rkvalvaa --expires 2027-01-31 --approved-by rkvalvaa
```

## How patterns are matched

QK110 follows the rules of [GitHub](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners#codeowners-syntax):

| Pattern | Matches |
| --- | --- |
| `*` | every file |
| `*.js` | every `.js` file, in any folder |
| `/docs/` | everything in `docs/` at the root |
| `docs/` | everything in any folder named `docs` |
| `docs/*` | files directly in `docs/`, not in its subfolders |
| `/apps/github` | the file or folder `apps/github` at the root |
| `**/logs` | every file or folder named `logs` |

Paths are case sensitive. `!` does not negate and `[ ]` is not a character range. Write a space in a path as `\ `. A line with an owner that is not a `@name` or an email address is skipped, as GitHub skips it.

On GitLab, two things differ:

- A path without a leading `/` matches in any folder: `docs/README.md` also matches `web/docs/README.md`.
- A section header such as `[Backend] @backend-team` gives its owners to the lines below it that name none.

## In the Stop hook

The Stop hook reports what the change did, so with `qk check --changed` QK110 checks only top-level paths the change adds. If the change touches `CODEOWNERS`, it checks every path. A gap that was there before the change is left for `qk check` and CI.

## Limits

- **QK110 does not check that an owner exists** or has write access. Your host does: GitHub shows errors when you open the file.
- **GitLab sections are read as one list.** A `!` exclusion, or a line without owners in a section without default owners, takes away an owner that another section gives.
- **Only GitHub and GitLab are known.** Other hosts that read `CODEOWNERS` are skipped.
- **Outside a git repository, QK110 checks nothing.**
