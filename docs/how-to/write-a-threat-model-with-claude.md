# Write a threat model with Claude

The `threat-model` skill writes a short threat model for a feature: where trust changes hands, what can go wrong there, what stops it, and what is still to do. Claude starts it when a new spec is added, when a change adds a trust boundary, when Quality Kit reports [QK040](require-a-threat-model-for-a-new-spec.md), or when you ask.

## Ask for it

In Claude Code, with the Quality Kit plugin enabled, name the spec or the feature:

```text
We have a new spec, docs/specs/checkout.md. Write its threat model.
```

You can also run `/quality-kit:threat-model docs/specs/checkout.md`.

## What Claude does

1. **Reads the feature**: the spec, the code the branch adds, and `quality.yaml`.
2. **Finds the trust boundaries**: a browser and your server, one tenant and another, your service and a provider, a webhook, a language model. It names the files where each is crossed.
3. **Walks STRIDE at each boundary**: spoofing, tampering, repudiation, information disclosure, denial of service, elevation of privilege. A threat gets a row only when it is real for this feature.
4. **Adds the LLM risks** when the feature sends text to a model or acts on its answer: the ten of the OWASP Top 10 for LLM applications, each with a row or a reason why it does not apply.
5. **Writes the file** to `docs/security/threat-models/<name of the spec>.md`, or to the folder under `security.threat_models` in `quality.yaml`, with `spec:` in its frontmatter so QK040 finds it.
6. **Runs `qk check`** and reports: the threats still to do, what it left out, and the decisions that wait for you.

## What is in the threat model

| Part | What it holds |
| --- | --- |
| **Trust boundaries** | At most five, each with what crosses it and the files where |
| **Threats** | Who does what and what it costs; the mitigation; a status; an ISO 27001 Annex A control |
| **LLM threats** | Only when the feature uses an LLM |
| **Mitigations to do** | A checklist, one line for each threat that is not mitigated yet |
| **Risks accepted** | Empty until a person signs one |

It keeps to two pages: at most 5 boundaries and 12 threats. When more are real, Claude keeps the costly ones and tells you what it left out.

## Status

- `in place`: the code shows the mitigation today, and the row names the file. A promise in the spec is not enough.
- `to do`: it does not yet. The row has a line in the checklist.

Claude never writes that a risk is accepted. If a threat is not worth its mitigation, write its ID, the reason, your name and the date under **Risks accepted**.

## Turn mitigations into tasks

The checklist in the file is the record, so nothing is lost when the session ends.

With Linear connected to Claude Code, Claude asks which mitigations to file and where. It creates an issue for each one you name, after your answer, and writes the issue's identifier at the end of the checklist line. It files nothing without an answer.

Without Linear, tick the lines off as the work is done.

## The control tags

Each threat names the Annex A control of ISO/IEC 27001:2022 that its mitigation serves, for example `A.8.24 Use of cryptography`. The threat model can then stand as evidence for that control. A new external service also gets a supplier control, A.5.19 to A.5.23.

## When the feature changes

Ask again. Claude keeps the IDs, the ticks and every line a person signed, and adds what is new.
