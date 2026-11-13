# Fill in quality.yaml after `qk init`

`qk init` wrote the answers and the folders. The keys below need facts only the repository holds.

The bar for every key: **the repository shows the fact**. Each path or glob you write matches a file that exists now (check with Glob), and each command is one the repository already runs. A key that fails the bar stays out of the manifest and goes on the hand-over list as left for the person.

## `docs.impact_map` (QK010)

Each entry says: when a file matching `paths` changes, a file matching `docs` changes in the same branch.

```yaml
docs:
  impact_map:
    - paths: ["internal/config/**"]
      docs: ["docs/reference/settings.md"]
```

- **Pair a code area with the doc that describes it today.** Typical pairs: settings code and the settings page; routes or handlers and the API guide; CLI commands and the CLI page; an admin UI and its how-to folder.
- **Start with one to three entries.** Each one blocks at silver and gold, so a wrong pair costs the team a waiver.
- **An area with no doc gets no entry.** Name the area in the hand-over instead.
- **Globs:** `*` matches within one folder level, `**` any number of levels, `?` one character. Everything else is literal, including `[id]` and `(admin)`.
- **Globs cannot exclude.** Where tests sit beside the code, a test-only change then also asks for docs; say so in the hand-over.
- When you add real entries, delete the commented example `qk init` wrote.

## `generated` (QK020)

A file that a command writes and git tracks.

```yaml
generated:
  - file: docs/reference/settings.md
    cmd: go run ./cmd/gen-settings-doc
```

- Find the command where the repository keeps it: `Makefile`, `package.json` scripts, `//go:generate` lines, the README.
- `cmd` runs from the repository root without a shell and is split on spaces. A command with a pipe, a redirect or a quoted argument needs a script; leave it for the person.
- A file under `generated` needs no `impact_map` entry.

## `contracts.openapi` (QK030)

List the OpenAPI files that exist. Look for `openapi*` and `swagger*` with `.yaml`, `.yml` or `.json`, outside `node_modules/` and `vendor/`.

```yaml
contracts:
  openapi: ["api/openapi.yaml"]
```

## `security.specs_dir` (QK040)

`qk init` wrote `docs/specs`. If the repository keeps specs, RFCs or design docs in another folder, name that folder. A new Markdown file there then needs a threat model.

## `owners`

```yaml
owners: { product: <person>, tech: <handle> }
```

Write it only when the user named the owners in this conversation. An existing `CODEOWNERS` line shows who reviews a path, which is a different fact.

## `.github/CODEOWNERS`

`qk init` wrote `@your-team` when the repository had no `CODEOWNERS`. Replace it only with an owner the user named. Otherwise it goes on the hand-over list.
