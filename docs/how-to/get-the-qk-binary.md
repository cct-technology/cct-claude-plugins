# Get the qk binary

Quality Kit's hooks need the `qk` binary. The plugin ships a small script, `bin/qk`, that finds the right one. Usually you do nothing: the first hook that needs `qk` downloads it.

## How the plugin finds qk

The required version is the `version` in the plugin's `plugin.json`. `bin/qk` tries, in order:

1. A `qk` on `PATH` at exactly that version.
2. A cached copy: `$CLAUDE_PLUGIN_DATA/<version>/qk` inside Claude Code, `~/.cache/quality-kit/<version>/qk` outside it.
3. A download of the matching release from GitHub, verified against the release's `checksums.txt`, then cached.

If all three fail, it prints a warning on stderr and exits 0. The check does not run and nothing is blocked. A download whose checksum does not match is refused and never cached.

## Use qk on a machine with no access to GitHub

Download the archive for your system from the [releases page](https://github.com/rkvalvaa/cct-claude-plugins/releases), for the version in `plugin.json`. Verify it against `checksums.txt`, then put `qk` on `PATH`:

```sh
sha256sum --ignore-missing -c checksums.txt
tar -xzf qk_<version>_linux_amd64.tar.gz qk
mv qk ~/.local/bin/
qk version
```

`qk version` must print the version in `plugin.json`, or the plugin ignores that binary. After a plugin update, install the matching `qk` again.

## Requirements

- macOS, Linux or Windows, on amd64 or arm64.
- `curl`, `tar` and `sha256sum` or `shasum` for the download.
- On Windows, [Git Bash](https://git-scm.com/downloads/win). `bin/qk` is a shell script, and without Git Bash Claude Code runs hooks in PowerShell, which cannot run it.
