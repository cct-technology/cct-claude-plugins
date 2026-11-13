---
name: quality-init
description: Set up Quality Kit in a repository. Asks for data class, maturity and replaced tools, runs `qk init`, and fills in `quality.yaml` from what the repository shows. Use when the user asks to set up, install, add or adopt Quality Kit or `qk` in a repo, or asks how to get a `quality.yaml`. Not for changing an existing `quality.yaml` (maturity, modes, waivers), and not for fixing findings.
---

# Set up Quality Kit

`qk` writes the files and decides every pass and fail. You ask, run it, and add what only this repository can say. Every fact you report is a line `qk` printed or a file you read.

`quality.yaml` is guarded: a shell command that names it stops for a person's approval, even one that only reads it. Find it with Glob, read it with Read, change it with Edit, and run each `qk` command on its own.

## 1. Is it set up already?

Look for `quality.yaml` at the repository root with Glob.

- **It exists:** the repository is set up. Run `qk check`, show the findings as `qk` printed them, and finish there. `quality.yaml` stays as it is: changing it is a person's decision.
- **It does not:** go on.

## 2. Ask once

Read the README and the top-level folders, enough to recommend an answer to each question. Then ask, in a single AskUserQuestion call, the questions the user has not already answered:

1. **Data class**: the most sensitive data the service handles. `internal`, `client` or `public`.
2. **Maturity**: which checks block. `bronze` for a legacy or adopted repository: every check warns. `silver` for a new service: docs impact, contracts, waivers, generated files and supply chain block. `gold` for a client-facing or regulated service: every check blocks.
3. **Replaced tools**: the tools this system takes over from, or none.

These three are the whole interview. Done when you hold all three answers.

## 3. Run `qk init`

```sh
qk init --data-class <class> --maturity <level> --replaces "<tool>"
```

Repeat `--replaces` for each tool, and leave it out for none. Keep the output: it is the record of the stack, the host, and each file `wrote`, `skipped` or `changed`. A `skipped` file belongs to the repository and stays as it is.

If the output says `qk did not run`, report that line and finish: nothing was set up.

## 4. Fill in the manifest

Follow [references/fill-in-the-manifest.md](references/fill-in-the-manifest.md) for `quality.yaml` and the CODEOWNERS stub. Its bar: write a key only when the repository shows the fact.

Your edit to `quality.yaml` stops for the person's approval, which is intended. When the approval does not come, leave the file and put each entry you would have written on the hand-over list, ready to paste.

Done when every key in the reference is either written from a fact you can point to, or on the hand-over list.

## 5. Check and hand over

Run `qk check`. Then report, in this order:

1. What `qk init` wrote, skipped and changed, from its output.
2. What you added to `quality.yaml`, and the file or folder each entry rests on.
3. The summary line of `qk check` and each finding, as printed.
4. The last paragraph of the `qk init` output, word for word: the setting that makes the host require review.
5. **Left for you:** what only a person can supply. Owners for CODEOWNERS, the keys you left out, and the `QK_RELEASE_TOKEN` secret when a CI workflow was written.
