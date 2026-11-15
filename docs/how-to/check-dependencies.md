# Check new and upgraded dependencies

QK060 checks every dependency a change adds or moves to another version, before it reaches your default branch.

A release that is a day old has been read by nobody. It may be a hijacked version of a package you have used for years, or a name an agent invented and an attacker registered. So a version has to be 14 days old before it comes in.

## What is checked

For each dependency that is new, or at a new version, `qk` asks the public registry four things:

| Question | A finding when |
| --- | --- |
| Does it exist? | The registry has no such package. |
| Is the version old enough? | It was published less than 14 days ago. |
| Is it maintained? | It is deprecated (npm, Go), abandoned (Packagist) or yanked (crates.io, PyPI), or its PyPI project is marked inactive. |
| Is its licence allowed? | It declares no licence, or one that is not MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD or Unlicense. |

It reads `package.json`, `go.mod`, `composer.json`, `Cargo.toml`, `pyproject.toml`, `Pipfile` and `requirements*.txt`, and asks npm, the Go module proxy, Packagist, crates.io and PyPI.

For a choice of licences, such as `MIT OR GPL-3.0-only`, one allowed licence is enough. For `MIT AND Zlib`, both must be allowed.

## Which version is checked

The one that gets installed, which is the one in the lockfile. `qk` reads `package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `composer.lock`, `Cargo.lock`, `poetry.lock`, `uv.lock` and `Pipfile.lock`. A `go.mod` names exact versions itself.

So an upgrade is found both ways:

- **You change the manifest**, for example `"react": "^18.3.1"` to `"^19.0.0"`.
- **Only the lockfile changes**, as after `npm update` or a bot's pull request. The range in `package.json` is the same, and the version behind it is new.

A dependency is checked when it resolves to a version it did not resolve to before. A new range that still gives the same version is not a change.

Without a lockfile, or for a name the lockfile does not hold yet, `qk` takes the version the manifest names when it names one exactly, such as `1.3.0` or `==1.3.0`. For a range, such as `^1.3.0`, it takes the newest release: a range is settled when the package is installed, and that is the version an install may fetch. Each finding names the version it looked at.

## Run it

QK060 uses the network, so it is one of the slow checks. It runs in CI, and when you ask for it:

```sh
qk check --full
```

Without `--full` it is skipped and names what it did not check:

```text
SKIP  QK060  New and upgraded dependencies exist, are old enough, maintained and licensed (enforced)
      skipped: slow: asks the package registries about `left-pad` (npm) 1.3.0, `is-odd` (npm) 3.0.1; runs with `qk check --full` or in CI
```

It does not run when Claude finishes. In a session, a hook covers the moment that matters most: when Claude installs a package, the install is [blocked](work-with-the-hooks.md#when-an-install-is-blocked) if the package does not exist or is too new. Nothing needs to be configured. From `silver` up, QK060 blocks. At `bronze` it warns.

## Read a finding

```text
FAIL  QK060  New and upgraded dependencies exist, are old enough, maintained and licensed (enforced)
      - package.json: `reakt-domm` (npm) does not exist in the registry
      - package.json: `left-pad` (npm) 1.3.0 is deprecated. The registry says: "use String.prototype.padStart()"
      - package.json: `left-pad` (npm) 1.3.0 has the licence `WTFPL`, which is not on the allowed list (MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD, Unlicense)
```

A version that is too young reads:

```text
      - package.json: `react` (npm) 19.0.0 was published on 2026-11-11, 3 days ago; a new version must be at least 14 days old
```

| The finding says | What to do |
| --- | --- |
| does not exist in the registry | Check the name. It may be invented or misspelt. Do not install it. |
| was published ... ago | Wait until it is 14 days old, or stay on an older version. |
| is deprecated, abandoned or yanked | Pick the package it names as its replacement, or another one. |
| has the licence ..., or declares no licence | Pick another package, or ask the person who decides about licences. |

An upgrade gets all four questions, not only the age. So moving an old dependency to a new version also reports that it is deprecated, or that its licence is not allowed, if nobody has waived that before.

## Check one package before you add it

```sh
qk deps verify npm left-pad
qk deps verify npm @types/node@22.1.0
qk deps verify go github.com/spf13/cobra@v1.8.0
```

The first word is the registry: `npm`, `go`, `composer`, `cargo` or `pypi`. Without a version, the newest release is checked.

```text
- `left-pad` (npm) 1.3.0 is deprecated. The registry says: "use String.prototype.padStart()"
- `left-pad` (npm) 1.3.0 has the licence `WTFPL`, which is not on the allowed list (MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD, Unlicense)
```

Exit code `1` means a finding. `0` means the package is fine, or the registry could not be asked: the output then starts with `skipped:`.

## When the dependency is right all the same

An urgent security patch may be a day old. A person can accept that with a [waiver](waive-a-finding.md) for that package and that version:

```sh
qk waiver add QK060 \
  --reason "express 4.19.3 is an urgent security fix; diff reviewed" \
  --owner rkvalvaa \
  --expires 2026-11-28 \
  --package express@4.19.3 \
  --approved-by rkvalvaa
```

The waiver covers `express` at 4.19.3 and nothing else. Another package in the same manifest is still checked, and so is `express` at 4.19.4.

Leave the version out to cover every version of a package, as for a package of your own in a private registry:

```sh
qk waiver add QK060 \
  --reason "Our own package, on our own registry" \
  --owner rkvalvaa \
  --expires 2027-06-30 \
  --package @acme/tools \
  --approved-by rkvalvaa
```

A waiver with `--path package.json` and no `--package` covers every QK060 finding in that manifest. Avoid it: it also lets through a package nobody looked at.

## When the registry cannot be reached

A registry that is down, slow or blocked by a firewall is never a finding. QK060 is skipped and names the package it could not ask about. Run it again later; CI runs it on every pull request.

Answers are kept for 24 hours in your cache folder, so a second run asks nothing. A package that was not found is asked about again every time.

## What it does not check

- **Private packages.** Only the public registries are asked, so a package of your own in a private registry is reported as missing. Waive that package by name, as shown above. The name of each dependency that is checked is sent to the public registry.
- **Dependencies of dependencies.** Only what your manifest names. A new version of a package that another package pulls in is not looked at.
- **`bun.lock`.** A Bun project is judged by its `package.json`.
- **Licences of Go modules.** The Go module proxy holds none. The other three questions are asked.
- **Indirect Go requirements, paths, git URLs and workspace members.** They do not come from a registry.

In a workspace with one lockfile, a package that two members hold at different versions is checked under each member that names it.

PyPI projects write their licence in many ways. `qk` reads the licence expression, then a short licence name, then the classifiers. A project that only says `BSD License` does not say which one, so it is a finding for a person to read.
