# Sourced by each case's scaffold.sh. Copies the qk on your PATH into this
# run's cache, where the plugin's bin/qk shim looks for it.
#
# Why: the agent's shell runs in a sandbox that sees the run's own home folder
# and the plugin, but not the folder your qk is in. See
# docs/how-to/run-the-skill-evals.md.

here=$(dirname "$0")
version=$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$here/../../.claude-plugin/plugin.json" | head -n 1)

qk_bin=$(command -v qk) || {
	echo "install-qk: no qk on PATH. Build one with the plugin's version first." >&2
	exit 1
}
if [ "$(head -c 2 "$qk_bin")" = "#!" ]; then
	echo "install-qk: the first qk on PATH is the plugin's shim ($qk_bin). Put the folder of the built qk before it." >&2
	exit 1
fi
case $("$qk_bin" version) in
"qk $version "*) ;;
*)
	echo "install-qk: $qk_bin is not qk $version, the version in plugin.json. Build it with -ldflags \"-X main.version=$version\"." >&2
	exit 1
	;;
esac

mkdir -p "$HOME/.cache/quality-kit/$version"
cp "$qk_bin" "$HOME/.cache/quality-kit/$version/qk"
