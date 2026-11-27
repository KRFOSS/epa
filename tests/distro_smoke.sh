#!/usr/bin/env bash
# Run only as root inside a disposable Linux container.
set -euo pipefail
export LC_ALL=C
export XDG_CONFIG_HOME
XDG_CONFIG_HOME=$(mktemp -d)
trap 'rm -rf -- "$XDG_CONFIG_HOME"' EXIT
unset SUDO_USER
bin=${1:?EPA binary required}
backend=${2:?expected backend required}
run() { "$bin" --style apt --progress native "$@"; }
"$bin" backend info | grep -F "Backend: $backend ("
case "$backend" in
 apt|dnf) run update ;;
 pacman) run full-upgrade -y ;;
 *) exit 2 ;;
esac
run search tree > "$XDG_CONFIG_HOME/search.log"
grep -qi tree "$XDG_CONFIG_HOME/search.log"
if command -v tree; then
    echo 'smoke test requires tree to be absent before installation' >&2
    exit 1
fi
run install tree -y
command -v tree
"$bin" __complete -- remove tre | grep -Fx tree
run remove tree -y
if command -v tree; then
    echo 'tree is still installed' >&2
    exit 1
fi
printf 'Native %s refresh/upgrade, search, install, completion and remove passed\n' "$backend"
