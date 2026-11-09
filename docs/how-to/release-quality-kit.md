# Release Quality Kit

A release publishes `qk` binaries and tells existing plugin installs to update. One version number drives both.

1. Raise `version` in `plugins/quality-kit/.claude-plugin/plugin.json`. Claude Code only updates an installed plugin when this changes, and `bin/qk` downloads the release named after it. Raise the `qk-version` default in `.github/workflows/quality.yml` to match; a test fails while they differ.
2. Move the `## Unreleased` entries in `CHANGELOG.md` under a heading for the new version.
3. Merge that change to `main`.
4. Tag the merge commit `v` plus the same version, and push the tag:

   ```sh
   git tag v0.2.0
   git push origin v0.2.0
   ```

The tag starts `.github/workflows/release.yml`. It fails if the tag does not match `plugin.json`, runs the tests, then GoReleaser publishes six archives (darwin, linux and windows on amd64 and arm64) and `checksums.txt`.

Until the release is published, `bin/qk` at the new version cannot download `qk`, so it warns and exits 0.

## Try the build without publishing

```sh
go run github.com/goreleaser/goreleaser/v2@v2.20.0 release --snapshot --clean
```

The archives and `checksums.txt` land in `dist/`. Asset names are a contract with `bin/qk`: change `.goreleaser.yaml` and the script together.
