# Set up Quality Kit in a repository

`qk init` writes what a repository needs to start: the manifest, the docs folders it names, and what your git host needs. It asks no questions and never overwrites a file, so you can run it in a repository that already has docs.

## Run it

From anywhere in the repository:

```sh
qk init --data-class client --maturity silver --replaces "Old CRM"
```

- `--data-class`: the most sensitive data the service handles: `internal` (the default), `client` or `public`.
- `--maturity`: which checks block: `bronze` (the default), `silver` or `gold`. See [Configure quality.yaml](configure-quality-yaml.md).
- `--replaces`: a tool the system replaces. Repeat the flag for each tool, and leave it out when the system replaces nothing.

## What it writes

Every repository gets:

| File | What it is |
| --- | --- |
| `quality.yaml` | The manifest, with your answers and the folders below. `service` is the name of the repository's folder. |
| `docs/tutorials/`, `docs/how-to/`, `docs/reference/`, `docs/explanation/` | The Diátaxis folders, each with a `README.md` that says what belongs there. |
| `docs/security/threat-models/` | Where threat models go, with a `README.md`. |
| `docs/testing/uat/` | Where UAT plans go, with a `README.md`. |
| `.gitattributes` | The line `.quality/exceptions-log.jsonl merge=union`. Two branches that each record an exception then merge without a conflict. If you have a `.gitattributes`, the line is added to it. |

The manifest also names `docs/specs` as the folder where a new spec needs a threat model ([QK040](require-a-threat-model-for-a-new-spec.md)). `qk init` does not make that folder. Change `security.specs_dir` if your specs live somewhere else.

When the `origin` remote is on GitHub, `qk init` also writes:

| File | What it is |
| --- | --- |
| `.github/CODEOWNERS` | A stub that gives `/.quality/` and `/quality.yaml` to `@your-team`. It is not written when GitHub already reads a `CODEOWNERS` file in `.github/`, the root or `docs/`: `qk init` then tells you to give the two paths an owner in yours. |
| `.github/workflows/quality-kit.yml` | A workflow that calls the [reusable workflow](run-quality-kit-in-github-actions.md) at the tag of the `qk` you ran. |

On another host, or without an `origin` remote, `qk init` writes the first group only and says so. Run it again once the remote is set.

## Read the report

```text
Stack: Go, Node.js
Host: GitHub
wrote docs/explanation/README.md
skipped docs/how-to/README.md: it exists
wrote quality.yaml
changed .gitattributes: added `.quality/exceptions-log.jsonl merge=union`
skipped .github/CODEOWNERS: `CODEOWNERS` exists; give `/.quality/` and `/quality.yaml` an owner in it, so a change to an exception needs a person's review
wrote .github/workflows/quality-kit.yml

To make GitHub require a person's review of `.quality/` and `quality.yaml` before a merge: ...
```

- **`Stack`** names what the repository is built with, from its `go.mod`, `package.json`, `composer.json`, `Cargo.toml`, `pyproject.toml`, `Pipfile` or `requirements.txt`. Files under `vendor/` and `node_modules/` do not count.
- **`wrote`** is a new file. **`skipped`** is a file that exists and was left as it is. **`changed`** is only ever `.gitattributes`.
- **The last paragraph** is the setting that makes the host require a person's review of `.quality/` and `quality.yaml` before a merge. Only the host can require that. On a host without an add-on, `qk init` says it knows no such setting.

A second run writes only what is still missing. On a repository that is already set up, it changes nothing.

## Finish the setup

1. In `quality.yaml`, add `owners`, and a `docs.impact_map` that says [which docs change with which code](keep-docs-in-step.md).
2. In `.github/CODEOWNERS`, replace `@your-team`, and [give the other top-level paths an owner](give-every-path-a-code-owner.md).
3. Turn on the setting `qk init` printed. For GitHub, the steps are under [Require a person's review of exceptions](run-quality-kit-in-github-actions.md#require-a-persons-review-of-exceptions).
4. While the Quality Kit repository is private, add the `QK_RELEASE_TOKEN` secret, as [Run Quality Kit in GitHub Actions](run-quality-kit-in-github-actions.md#while-the-quality-kit-repository-is-private) describes.
5. Run [`qk check`](run-qk-check.md).
