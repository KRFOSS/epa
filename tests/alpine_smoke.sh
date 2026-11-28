#!/bin/sh
# Run as root only in a disposable Alpine container.
set -eu
export LC_ALL=C
export XDG_CONFIG_HOME
XDG_CONFIG_HOME=$(mktemp -d)
trap 'rm -rf -- "$XDG_CONFIG_HOME"' EXIT
unset SUDO_USER SUDO_COMMAND
bin=${1:?EPA binary required}
run() { "$bin" --style apt --progress native "$@"; }
apk --version
"$bin" backend info | grep -F "백엔드: apk ("
"$bin" style set apk
"$bin" style current | grep -Fx apk
"$bin" __complete -- ad | grep -Fx add
"$bin" __complete -- cache "" | grep -Fx clean
"$bin" style reset
"$bin" style current | grep -Fx apt

# A failed or interrupted refresh must not be mistaken for success.
run update
"$bin" --style pacman --progress native -Syy
run search tree | grep -F tree
run show tree | grep -F tree
"$bin" __complete -- install tre | grep -Fx tree
"$bin" __complete -- --style apk add tre | grep -Fx tree
run list | grep -Fx busybox
run outdated
run --dry-run install tree | grep -F '"--interactive"'
run --dry-run install tree -y | grep -F '"--no-interactive"'
run --dry-run upgrade | grep -F '"upgrade"'
"$bin" --style pacman --progress native --dry-run -Syyu | grep -F '"--force-refresh"'
if apk info -e tree >/dev/null; then
    echo 'tree already installed' >&2; exit 1
fi
if run install tree --allow-untrusted; then exit 1; fi
if run autoremove; then exit 1; fi
if "$bin" --style pacman --progress native -R tree; then exit 1; fi

# Test both command styles against actual world transactions.
"$bin" --style apt --progress plain install tree -y
apk info -e tree
command -v tree
"$bin" __complete -- remove tre | grep -Fx tree
"$bin" --style apk --progress native fix tree --no-interactive
"$bin" --style apk --progress native del tree --no-interactive
if apk info -e tree >/dev/null; then exit 1; fi
"$bin" --style apk --progress native add tree --no-interactive
run remove tree -y
if apk info -e tree >/dev/null; then exit 1; fi
"$bin" --style apk --progress native cache clean
run upgrade -y
printf '%s\n' 'APK refresh, search, info, completion, install, reinstall, remove, and upgrade passed'
