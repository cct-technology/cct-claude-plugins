# qk decides, Claude writes

Quality Kit splits the work in two. A program decides whether something is missing. A model writes what is missing. This page says why.

## The problem with asking the model

An agent that has just written code is a poor judge of whether its docs are current. It tends to say yes. Ask it twice and you may get two answers. And when it is wrong, nothing shows why.

A gate that sometimes opens by itself is not a gate. So the decision is not the model's to make.

## What `qk` decides

`qk` is a plain program. Given the same repository and the same base, it gives the same answer every time. Each check returns `pass`, `warn`, `fail` or `skipped` with a reason, and nothing else decides whether a change blocks.

The checks ask questions a program can answer for certain:

- Did a file under these paths change, and did a mapped doc change too? (QK010)
- Does this file equal what its generator writes? (QK020)
- Does `oasdiff` find a breaking change between the base and now? (QK030)
- Does this new spec have a threat model with its name? (QK040)

`qk` does not read a doc and judge whether it is good. It checks that the doc was touched, exists, or matches. That is a weaker question than "is it right", and it is one with a certain answer.

## What Claude does

The skills do what a program cannot: read the diff, understand what changed for a reader, and write the how-to, the contract note, the threat model or the test plan.

A skill takes its facts from `qk`. `docs-sync` starts from the findings of `qk check`, not from its own view of what is stale. `contract-guard` has `qk` compare the contract with the base, and when no comparison ran, it says so and gives its own reading as a reading.

A skill never decides pass or fail. After it writes, `qk` runs again, and that result stands.

## One binary, three callers

The hooks, your terminal and CI all run the same `qk`. A check that passes when Claude finishes passes in the pull request, and the reverse. There is no second implementation that could disagree.

## Reference is generated, not written

A model can write a tutorial or a how-to. It must not write reference facts: the list of settings, the rules, the keys of a file. Those are generated from the code, and QK020 fails when a generated file differs from what its generator writes.

The [rule reference](../reference/rules.md) and the [`quality.yaml` reference](../reference/manifest.md) of Quality Kit itself are made this way.

## Exceptions need a person

Sometimes a finding should not be fixed now. That is a decision about risk, so a person makes it. Claude can propose a waiver or a "no docs needed" note, and Claude Code then asks a person to approve it. The approval is logged, and a waiver that was written by hand or edited afterwards does not count.

See [Waive a finding](../how-to/waive-a-finding.md).

## Not grading your own work

The same idea applies to review. A session that wrote the code should not be the one that reviews its artifacts. The specification has a reviewer agent for this, with read-only tools and a fresh context. It is planned for a later phase and not built yet: see [`SPEC.md`](../SPEC.md#agent).

## What this costs

`qk` can be satisfied by a doc change that says nothing useful. It checks that the work was done, not how well. Review by a person at merge is still needed. Quality Kit makes sure there is something to review.
