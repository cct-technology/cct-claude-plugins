# Require a threat model for a new spec

QK040 checks that every new spec comes with a threat model, so security is thought through when a feature is specified, not after it ships.

## Name your folders

In `quality.yaml`:

```yaml
version: 1
maturity: gold
security:
  specs_dir: docs/specs
  threat_models: docs/security/threat-models
```

- `specs_dir` is the folder your specs live in. Leave it out and QK040 checks nothing.
- `threat_models` is the folder your threat models live in. Leave it out and QK040 looks in `docs/security/threat-models`.

At `gold`, QK040 blocks. At `bronze` and `silver`, it warns. To make it block at `silver`, set `security.threat-model: enforced` under `modes`. The check key is `security.threat-model`.

Without a `quality.yaml`, QK040 checks `docs/specs` if that folder exists, and only warns.

## Give a spec its threat model

A threat model belongs to a spec in one of two ways:

- **Same file name.** `docs/specs/payments.md` is covered by `payments.md` anywhere under the threat model folder.
- **Frontmatter.** The threat model names the spec, with its path from the repository root:

  ```markdown
  ---
  spec: docs/specs/payments.md
  ---
  # Card data
  ```

The threat model can be part of the same change as the spec, or already be there.

## Read the result

```text
FAIL  QK040  A new spec has a threat model (enforced)
      - docs/specs/payments.md: new spec without a threat model: add `docs/security/threat-models/payments.md`, or put `spec: docs/specs/payments.md` in the frontmatter of a threat model under `docs/security/threat-models`
```

If the spec needs no threat model, ask a person for a [waiver](waive-a-finding.md):

```sh
qk waiver add QK040 --path docs/specs/payments.md --reason "Copy change only, no new data or boundary" --owner rkvalvaa --expires 2026-12-31 --approved-by rkvalvaa
```

## Limits

- **Only new specs are checked.** Editing a spec, or moving one into the folder, needs nothing.
- **A spec is a Markdown file** (`.md`). Other new files in the specs folder, such as images, are ignored.
- **One `spec:` per threat model.** A threat model that covers several specs needs the same file name as one of them, or one threat model per spec.
- **QK040 does not read the threat model.** It checks that one exists, not what it says.
