# Catch breaking API changes

QK030 compares each changed OpenAPI file with its version at the base, and fails when the change breaks clients. It uses [oasdiff](https://github.com/oasdiff/oasdiff).

To have Claude update the contract for a change and bring a break to you, see [Keep contracts true with Claude](keep-contracts-true-with-claude.md).

## List your OpenAPI files

In `quality.yaml`:

```yaml
version: 1
maturity: silver
contracts:
  openapi: ["api/openapi.yaml"]
```

Each entry is a glob: `*` matches within one folder level, `**` matches any number of levels.

From `silver` up, QK030 blocks. At `bronze`, it warns. The check key is `contracts.breaking`.

## Install oasdiff

```sh
go install github.com/oasdiff/oasdiff@latest
```

Other ways to install it are in the [oasdiff README](https://github.com/oasdiff/oasdiff#installation). Use version 1.12 or newer: QK030 asks oasdiff to read the base version from git.

Without `oasdiff` on `PATH`, QK030 is skipped and says so. A skipped check never blocks, so install oasdiff wherever the check should count, CI included. For the GitHub Actions workflow, see [OpenAPI contracts](run-quality-kit-in-github-actions.md#openapi-contracts).

## Read the result

```text
FAIL  QK030  No breaking OpenAPI changes (enforced)
      - api/openapi.yaml: GET /orders: added the new required `query` request parameter `tenant` (new-required-request-parameter)
      - api/openapi.yaml: GET /orders/{id}: api path removed without deprecation (api-path-removed-without-deprecation)
```

- **A breaking change** is a finding, one per change. The name in brackets is oasdiff's ID for the kind of change.
- **A change oasdiff cannot confirm as breaking**, such as a removed request parameter, is a warning that starts with `possibly breaking`.
- **A deleted contract** fails.
- **A new contract** passes: there is nothing at the base to break.
- **A renamed contract** is compared with what it held under its old name.

## Fix a finding

Keep the API compatible: put back what was removed, or make the new requirement optional.

If the break is intended, release it as a new API version. To accept it for a set time, ask a person for a [waiver](waive-a-finding.md):

```sh
qk waiver add QK030 --path api/openapi.yaml --reason "v1 has no clients yet" --owner rkvalvaa --expires 2026-12-31 --approved-by rkvalvaa
```

## Limits

- **Only a listed file starts a comparison.** If your contract is split over several files with `$ref`, list the root file. A change in a file it refers to is compared when the root file changes too.
- **Multi-file contracts are skipped on Windows.** oasdiff cannot read the base version of a `$ref` target there (seen in oasdiff 1.35 and 1.37). Linux and macOS compare them, so run the check in CI.
- **A contract oasdiff cannot read is skipped**, with what oasdiff said. Fix the file and run `qk check` again.
