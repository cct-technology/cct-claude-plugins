# Write a UAT plan with Claude

The `test-plan` skill writes the plan people follow when they test a release by hand: steps for each role, exploratory charters, security checks, and a place to record what each tester saw. Claude starts it when you say a release or an epic is ready for testing, or when you ask for a UAT plan.

## Ask for it

In Claude Code, with the Quality Kit plugin enabled, name the release:

```text
Release 2.3.0 is ready for people to test. Write the test plan.
```

You can also run `/quality-kit:test-plan 2.3.0`.

## What Claude does

1. **Reads what the release holds**: everything since the last tag, with the changelog, docs and specs that describe it. For an epic on a branch, everything since the branch left the default branch.
2. **Finds who uses it**: the roles the code grants and refuses, whether accounts belong to organisations that must not see each other's data, and when a session ends. It uses each name as the repository writes it. When the repository does not say who uses a change, Claude asks you.
3. **Writes the plan** to `docs/testing/uat/<release>.md`, or to the folder under `testing.uat_dir` in `quality.yaml`.
4. **Runs `qk check`** and reports: where the plan is, what it left to automation, and what a tester needs before starting.

Claude writes the plan. It does not run it: every row starts as `todo`.

## What is in the plan

| Part | What it holds |
| --- | --- |
| **Before you start** | The accounts, organisations and data a tester needs |
| **Steps for each role** | One action and one expected result in a row. Only roles whose work the release changes |
| **Exploratory charters** | One to three aims for testing off the path, each with a time |
| **Security acceptance checks** | Wrong role, another tenant, expired session |

Claude leaves out what an automated test in the repository already checks, and tells you which tests those are.

A security check that does not apply is not dropped in silence: the plan has one line under the table that says why. In a system where every account has the same rights, that line is "Wrong role: does not apply".

The plan keeps to two pages: at most 8 steps for a role, 3 charters and 6 security checks.

Claude does not know the address of your test system, who tests or when. Add those yourself.

## Record what you saw

Each row has a **Status** and an **Evidence** cell.

1. Run the row.
2. Set **Status** to `pass`, `fail` or `blocked`, in lower case, alone in the cell.
3. Put what you saw under **Evidence**: a screenshot, a link to an issue, or a sentence. A `fail` or `blocked` row needs evidence.
4. Write your name and the date under the table.

Keep the columns and their order. A later check, QK050, will read the **Status** column when the release is tagged, and a row still `todo` will count as not run.

## When there is nothing to test by hand

A release that only moves code, adds tests or changes the build shows a person nothing new. The plan then has one section, **Nothing to test by hand**, with what the release changes, why nobody can see it, and the tests that cover it. Sign under **Agreed by** when you agree. If you do not, tell Claude what a person would notice and ask again.

## When the plan exists already

Ask again after the release changed. Claude keeps the IDs, and the status and evidence of the rows people ran, and adds rows for what is new. It sets a row back to `todo` only when the release changed what the row tests, and says so.
