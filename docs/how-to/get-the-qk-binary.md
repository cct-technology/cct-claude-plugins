# Get the qk binary

Quality Kit's hooks need the `qk` binary. The plugin ships a small script, `bin/qk`, that finds the right one. Once the first release is out you do nothing: the first hook that needs `qk` downloads it.

> [!NOTE]
> Today you install `qk` yourself. The download needs a release, and there is none yet: `v0.1.0` is not tagged. See [Install qk yourself](#install-qk-yourself).

## How the plugin finds qk

The required version is the `version` in the plugin's `plugin.json`. `bin/qk` tries, in order:

1. A `qk` on `PATH` at exactly that version.
2. A cached copy: `$CLAUDE_PLUGIN_DATA/<version>/qk` inside Claude Code, `~/.cache/quality-kit/<version>/qk` outside it.
3. A download of the matching release from GitHub, verified against the release's `checksums.txt`, then cached.

If all three fail, it prints a warning on stderr and exits 0. The check does not run and nothing is blocked. A download whose checksum does not match is refused and never cached.

## Install qk yourself

Do this before the first release, or on a machine with no access to GitHub. `<version>` is the `version` in the plugin's `plugin.json`.

On Windows, run the commands in Git Bash, and write `qk.exe` wherever a command names the file `qk`. The plugin looks for that name, and does not find a file named `qk`.

**Before the first release**, build `qk` from a clone. This needs Go:

```sh
git clone https://github.com/cct-technology/cct-claude-plugins
cd cct-claude-plugins
go build -ldflags "-X main.version=<version>" -o qk ./cmd/qk
```

**From a release**, download the archive for your system and `checksums.txt` from the [releases page](https://github.com/cct-technology/cct-claude-plugins/releases).

Verify a downloaded archive against `checksums.txt` and unpack it:

```sh
sha256sum --ignore-missing -c checksums.txt
tar -xzf qk_<version>_linux_amd64.tar.gz qk
```

Then move `qk` to a folder on your `PATH`, such as `~/.local/bin`, and check it:

```sh
mv qk ~/.local/bin/
qk version
```

`qk version` must print the version in `plugin.json`, or the plugin ignores that binary. After a plugin update, install the matching `qk` again.

A `qk` built from source between releases carries the same version, so build it from a commit that has every hook the plugin calls. See [work with the hooks](work-with-the-hooks.md#use-a-qk-you-installed-yourself).

## Requirements

- macOS, Linux or Windows, on amd64 or arm64.
- `curl`, `tar` and `sha256sum` or `shasum` for the download.
- On Windows, [Git Bash](https://git-scm.com/downloads/win). `bin/qk` is a shell script, and without Git Bash Claude Code runs hooks in PowerShell, which cannot run it.
