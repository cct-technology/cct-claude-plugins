---
name: test-plan
description: Write the plan people follow when they test a release by hand. Use when an epic or release is ready for human testing, or when the user asks for a UAT plan, a test plan for testers, or acceptance steps. Reads what the release changes and who uses it, then writes `docs/testing/uat/<release>.md` with steps for each role, exploratory charters, security acceptance checks (wrong role, another tenant, expired session), and a status and evidence column for every row. Not for writing automated tests, and not for running the plan.
---

# Write the plan people test from

People test what automation cannot judge: whether the work can be done, makes sense, and holds when someone leaves the path. The plan gives each tester rows for their role and a place to record what they saw. Roles, screens and rules come from the repository or the person, never from you. You write the plan; people run it.

## 1. Read what the release holds

The release has the name the person gave it. Without a name, ask for it.

It holds everything since the release before it:

```sh
git describe --tags --abbrev=0
```

Then, with that tag written out:

```sh
git log --oneline <tag>..HEAD
git diff --stat <tag>
```

Without a tag, read the unreleased part of the changelog and the history. For an epic on a branch, the change is everything since the branch left the default branch. Read the changed files, and the changelog, docs and specs that describe them.

Done when you can list each thing a person can now do, or does differently, with the file that shows it.

## 2. Find who uses it

- **Roles:** the code that grants or refuses, and the docs that name roles. Use each name as it is written. One kind of user is one role.
- **Tenants:** whether an account belongs to an organisation, tenant or customer whose data the others must not see.
- **Sessions:** whether there is a sign-in, and when a session ends.
- **`quality.yaml`**, when there is one: `testing.uat_dir` (the folder for plans, `docs/testing/uat` without it), `testing.replaces` and `data_class`.

When the repository does not say who uses a change, ask the person. A role nobody has gives steps nobody can run.

## 3. When there is nothing a person can see

The list from section 1 is empty when the release moves code, adds tests or changes the build, and every screen, message and result stays the same. Then the plan is this, and nothing more:

```markdown
# UAT plan: <release>

## Nothing to test by hand

<What the release changes, and why no person can see it.>

**Covered by automation:** <the tests and checks that cover the change, each by its path>

**Agreed by:**
```

Leave **Agreed by** empty: the person who decides the release signs there. Go to section 5.

## 4. Write the plan

Read [references/what-to-test.md](references/what-to-test.md) first: it says which rows earn their place and how to word them.

Copy [template.md](template.md) to `<uat_dir>/<release>.md` and fill it:

- **Steps** for each role whose work the release changes. A role with no change gets no section.
- **Exploratory charters:** one to three.
- **Security acceptance checks:** wrong role, another tenant, expired session. Each one that applies gets a row with the names you found in section 2. For one that does not apply, write one line under the table that says why.
- Every row has the status `todo` and no evidence.

The comment at the top of the template is the budget. Keep to it, and leave the comment out of the plan.

When the plan exists already, update it: keep the IDs, and the status and evidence of the rows people ran. Add rows for what is new. Set a row back to `todo` only when the release changed what it tests, and say so in the report.

## 5. Check and report

```sh
qk check --format claude
```

1. Where the plan is, the roles it covers and how many rows each has.
2. What you left to automation, and the tests that cover it.
3. A security check that does not apply, and why.
4. What a tester needs before starting: the accounts, the organisations, the data.
5. Findings `qk` printed, as printed, and any question that waits for the person.
