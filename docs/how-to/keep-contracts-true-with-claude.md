# Keep contracts true with Claude

The `contract-guard` skill keeps your API, event and dataset contracts in line with the code, and stops for your decision when a change breaks clients. Claude starts it when a change alters what a client sends or gets back, when Quality Kit reports QK030, or when you ask.

## Ask for it

In Claude Code, with the Quality Kit plugin enabled, say what changed:

```text
The order endpoint now returns the currency too. Bring the contract in line.
```

You can also run `/quality-kit:contract-guard`.

## What Claude does

1. **Finds the contract**: the files under `contracts` in `quality.yaml`, and contract files in the repository that are not listed there.
2. **Reads the change**: everything since the branch left the default branch, committed or not.
3. **Makes the contract say what the code does.** It changes what the diff changed, with the names, types and required fields the code shows. A generated contract is regenerated with its command, never edited.
4. **Has `qk` compare** the contract with its version at the base: `qk check --format claude`, which runs [QK030](catch-breaking-api-changes.md) for OpenAPI files.
5. **Writes the note for clients**, for a change that does not break them.
6. **Reports**: what it changed and which part of the diff that rests on, what `qk` said about QK030, the note and where it is, and any question that waits for you.

The contract follows the code even when that breaks clients. Claude does not keep a field in the contract that the code no longer returns.

## When the change breaks clients

Claude tells you what breaks, in the words `qk` printed, lists the ways to stay compatible that fit, and asks which you intend:

- keep the old field beside the new one and mark it deprecated
- make a new requirement optional, with a default
- add a new version and leave the old one working
- deprecate now and remove later

Then it stops. It writes no waiver and no note for clients until you answer. If the Stop hook sends Claude back because QK030 blocks, it gives you the finding and the question again.

When you confirm the break is intended, Claude drafts the [waiver](waive-a-finding.md) for you:

```sh
qk waiver add QK030 --path api/openapi.yaml --reason "..." --owner <who> --expires <date> --approved-by <who>
```

It runs the command only after you say yes and give the names. Claude Code then asks you once more. A waiver accepts the finding for a time; the change still breaks clients, so the note for them says what they must do.

## When no comparison ran

`qk` compares OpenAPI files only, and only with `oasdiff` installed. In these cases Claude says that no automated comparison ran, and why:

- `oasdiff` is not installed, or cannot read the contract
- the contract is AsyncAPI or ODCS

It then gives its own reading of the diff, named as its reading. Treat that as advice, not as a check. To get the check, [install oasdiff](catch-breaking-api-changes.md#install-oasdiff).

## The note for clients

One entry for each change a client can see: what changed, and what a client has to do.

- With a `CHANGELOG.md` or `CHANGES.md`, Claude adds the entry under its unreleased heading.
- Without one, the note is in Claude's report, for you to put in the pull request.

## When there is no contract

Claude says which endpoint, event or dataset has no contract, and writes none: a contract made up from one diff would state facts nobody checked. Write or generate the file, list it under `contracts` in `quality.yaml`, and ask again.
