# Set up Quality Kit with Claude

The `quality-init` skill sets a repository up in one conversation. It asks up to three questions, runs [`qk init`](set-up-quality-kit-in-a-repository.md), and fills in `quality.yaml` from what the repository shows.

## Ask for it

In Claude Code, with the Quality Kit plugin enabled, say:

```text
Set up Quality Kit in this repository.
```

You can also run `/quality-kit:quality-init`.

Claude asks three questions in one go:

1. **Data class**: the most sensitive data the service handles: `internal`, `client` or `public`.
2. **Maturity**: which checks block: `bronze`, `silver` or `gold`. See [Configure quality.yaml](configure-quality-yaml.md).
3. **Replaced tools**: the tools this system takes over from, or none.

Give the answers in your request and Claude asks nothing:

```text
Set up Quality Kit here. Client data, silver, and it replaces our old CRM.
```

## What Claude does

1. **Runs `qk init`** with your answers. It writes the manifest, the docs folders and, on GitHub, the CODEOWNERS stub and the CI workflow. Files you already have are left as they are.
2. **Fills in `quality.yaml`** where the repository shows the fact:
   - `docs.impact_map`: a code area and the doc that describes it today.
   - `generated`: a file that a command in the repository writes.
   - `contracts.openapi`: the OpenAPI files that exist.
   - `security.specs_dir`: the folder your specs are in, when it is not `docs/specs`.
3. **Runs `qk check`** and shows the result as `qk` printed it.
4. **Hands over**: what was written and skipped, what it added and which file each entry rests on, the setting that makes your git host require review, and a list headed **Left for you**.

Claude writes a key only when a file in the repository backs it. It does not make up owners: `@your-team` in CODEOWNERS and `owners` in `quality.yaml` stay for you, unless you named them.

## Approve the change to quality.yaml

`quality.yaml` decides which checks block, so Claude Code stops and asks you before Claude changes it. Read the entries and approve them if they are right.

If you decline, Claude leaves the file and lists the entries under **Left for you**, ready to paste.

## In a repository that is already set up

When `quality.yaml` exists, the skill does not run `qk init` again. It runs `qk check` and shows the findings. To change the maturity or a check's mode, edit `quality.yaml` yourself.

## Afterwards

Work through **Left for you**. The steps are the same as in [Finish the setup](set-up-quality-kit-in-a-repository.md#finish-the-setup).
