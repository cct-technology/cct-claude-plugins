---
name: threat-model
description: Write the threat model for a feature. Use when a new spec is added, when a change adds a trust boundary (an external service, a webhook, an LLM, a new kind of user), when Quality Kit reports QK040, or when the user asks for a threat model. Finds the trust boundaries, walks STRIDE at each, adds the OWASP Top 10 for LLM applications when the feature uses an LLM, tags ISO 27001 Annex A controls, and writes `docs/security/threat-models/<feature>.md` in at most two pages, with the mitigations still to do as a checklist. Files them as Linear issues only after the person says which. Not for a penetration test, and not for a review of code that has no new boundary.
---

# Write the threat model

A threat model says what can go wrong where trust changes hands, and what is done about it. It is short so that people act on it. Boundaries and mitigations come from the spec and the code; whether to live with a risk is the person's decision, never yours.

## 1. Read the feature

- **The spec:** the file the person named, or the Markdown file the branch adds under `security.specs_dir` in `quality.yaml` (`docs/specs` without a `quality.yaml`). The threat model takes the spec's file name.
- **The code,** when there is some. The change is everything since the branch left the default branch:

  ```sh
  git diff --stat "$(git merge-base <default branch> HEAD)"
  ```

- **`quality.yaml`**, when there is one: `security.threat_models` (the folder, `docs/security/threat-models` without it) and `data_class`.

With no spec, name the threat model after the feature as the person called it.

Done when you can say who and what takes part, what data moves between them, and whether a language model reads or writes any of it.

## 2. Find the trust boundaries

A boundary is where data or a request passes between two sides that trust each other differently. [references/stride.md](references/stride.md) lists the usual ones. Take them from the spec and the code, and name the files where each is crossed.

## 3. Walk STRIDE at each boundary

At each boundary ask the six questions in [references/stride.md](references/stride.md). Write a row for a threat that is real for this feature, and leave out the ones that are not.

For each row:

- **Mitigation:** what stops it, in one sentence.
- **Status:** `in place` when the code shows it today, and then the mitigation names the file. Otherwise `to do`.
- **Control:** the Annex A control it serves, from [references/annex-a.md](references/annex-a.md).

## 4. When the feature uses an LLM

The feature uses an LLM when it sends text to a model, or acts on what a model returns. Then go through the ten risks in [references/llm-top-10.md](references/llm-top-10.md). Each one that applies gets a row. Name the others on the `Does not apply` line, each with its reason, so that all ten are accounted for.

Without an LLM, leave the section out.

## 5. Write the file

Copy [template.md](template.md) to `<threat_models>/<feature>.md` and fill it. The comment at its top is the budget: keep to it, and leave the comment out of the file. When more threats are real than the budget holds, keep the ones that cost most and say in the report what you left out.

Every row that is `to do` gets a line under **Mitigations to do**, unticked.

When the threat model exists already, update it: keep the IDs, the ticks, and any line a person signed.

## 6. Turn mitigations into tasks

The checklist in the file is the record. When a tool that creates Linear issues is connected, ask the person which of the mitigations to file, and where. Create an issue for each one they name, after their answer, and put its identifier at the end of the checklist line. Without an answer, file nothing.

## 7. Check and report

```sh
qk check --format claude
```

1. Where the threat model is, and the spec it belongs to.
2. The boundaries, and the threats that are `to do`, most costly first.
3. What you left out for the budget.
4. What `qk` printed about QK040, as printed.
5. The decisions that wait for the person: which mitigations to file, and any risk they may want to accept.
