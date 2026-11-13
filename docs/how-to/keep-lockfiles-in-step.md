# Keep lockfiles in step with dependencies

QK061 checks that every dependency manifest has a lockfile in git, and that the lockfile changes when the dependencies do. Without a lockfile, a build installs whatever versions are newest that day, not the ones you reviewed and tested.

QK061 needs no configuration. It only reads files; it never runs a package manager.

## What is checked

| Manifest | Lockfile |
| --- | --- |
| `package.json` | `package-lock.json`, `npm-shrinkwrap.json`, `pnpm-lock.yaml`, `yarn.lock`, `bun.lock` or `bun.lockb` |
| `go.mod` | `go.sum` |
| `composer.json` | `composer.lock` |
| `Cargo.toml` | `Cargo.lock` |
| `Pipfile` | `Pipfile.lock` |
| `pyproject.toml` with Poetry or uv | `poetry.lock` or `uv.lock` |

Two rules apply to each manifest git tracks or would add, in any folder:

- **A manifest that declares dependencies needs its lockfile in git**, in the same folder. For npm, pnpm, Yarn, Bun, Cargo and uv, a lockfile in a parent folder counts, because a workspace keeps one lockfile at its root.
- **When a manifest's dependencies change, the lockfile changes too**, in the same change set. Other edits to a manifest, such as a script, a version number or a comment, need nothing.

A manifest without dependencies needs no lockfile. Folders named `node_modules` or `vendor` are left out.

From `silver` up, QK061 blocks. At `bronze` and without a `quality.yaml`, it warns. The check key is `supply-chain.lockfile`.

## Read the result

```text
FAIL  QK061  Dependencies have a lockfile that changes with them (enforced)
      - go.mod: dependencies changed, but `go.sum` did not: update the lockfile and commit it
      - web/package.json: dependencies without a lockfile: commit `package-lock.json`, `npm-shrinkwrap.json`, `pnpm-lock.yaml`, `yarn.lock`, `bun.lock` or `bun.lockb`
```

## Fix a finding

Run your package manager so it writes the lockfile, then commit the lockfile with the manifest:

| Tool | Command |
| --- | --- |
| npm | `npm install` |
| pnpm | `pnpm install` |
| Yarn | `yarn install` |
| Go | `go mod tidy` |
| Composer | `composer update --lock` |
| Cargo | `cargo generate-lockfile` |
| Poetry | `poetry lock` |
| uv | `uv lock` |
| Pipenv | `pipenv lock` |

If the lockfile exists but QK061 reports it as missing, git ignores it. Take it out of `.gitignore`.

If a manifest should have no lockfile, such as a library that leaves it out on purpose or a test fixture, ask a person for a [waiver](waive-a-finding.md):

```sh
qk waiver add QK061 --path testdata/app/package.json --reason "Test fixture, never installed" --owner rkvalvaa --expires 2026-12-31 --approved-by rkvalvaa
```

To turn the check off for the whole repository, set `supply-chain.lockfile: "off"` under `modes` in `quality.yaml`.

## Python without a lock tool

`requirements.txt`, and a `pyproject.toml` that uses neither Poetry nor uv, have no lockfile to check. If that is all the repository has, QK061 is skipped and says so:

```text
SKIP  QK061  Dependencies have a lockfile that changes with them (advisory)
      skipped: Python dependencies without a lock tool (requirements.txt), so there is no lockfile to check; uv, Poetry and Pipenv write one
```

QK061 takes Poetry or uv to be in use when `pyproject.toml` has a `[tool.poetry]` or `[tool.uv]` table, or when a `poetry.lock` or `uv.lock` is there.

## Limits

- **QK061 does not check that the lockfile is right.** It checks that the lockfile changed when the dependencies did, not that it matches them. Your package manager checks that: run `npm ci`, `cargo build --locked` or `uv sync --locked` in CI.
- **Only declared dependencies are compared.** A change that alters the lockfile in another way, such as a new version number of your own package in `Cargo.toml`, is not seen.
- **Any lockfile in a parent folder counts** for the workspace tools. A package that is not a workspace member, below a folder that has a lockfile, passes without its own.
- **`Cargo.toml`, `pyproject.toml` and `Pipfile` are read line by line**, not as TOML. Tables such as `[dependencies]`, and keys such as `dependencies = [...]` under `[project]`, are found. A changed comment at the end of a dependency line counts as a change.
- **Without a base to compare with**, for example in a repository with no commits yet, QK061 only checks that each lockfile is there.
- **Outside a git repository, QK061 checks nothing.**
