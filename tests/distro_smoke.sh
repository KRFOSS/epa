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
# Fedora base images may already include tree. Never remove a package
# that was present before the smoke test.
package=tree
if [[ $backend == dnf ]]; then
    package=
    for candidate in tree ed figlet; do
        if rpm -q "$candidate" >/dev/null || command -v "$candidate" >/dev/null; then
            continue
        fi
        # A real repository error should fail the test, not skip a candidate.
        run search "$candidate" > "$XDG_CONFIG_HOME/search.log"
        if grep -Fqi -- "$candidate" "$XDG_CONFIG_HOME/search.log"; then
            package=$candidate
            break
        fi
    done
    if [[ -z $package ]]; then
        echo 'no uninstalled DNF smoke-test package found (tree, ed, figlet)' >&2
        exit 1
    fi
else
    run search "$package" > "$XDG_CONFIG_HOME/search.log"
    grep -Fqi -- "$package" "$XDG_CONFIG_HOME/search.log"
    if command -v "$package" >/dev/null; then
        printf 'smoke test requires %s to be absent before installation\n' "$package" >&2
        exit 1
    fi
fi
printf 'Testing native %s transaction with %s\n' "$backend" "$package"
run install "$package" -y
command -v "$package"
"$bin" __complete -- remove "${package:0:2}" | grep -Fx -- "$package"
run remove "$package" -y
if command -v "$package" >/dev/null; then
    printf '%s is still installed\n' "$package" >&2
    exit 1
fi
printf 'Native %s refresh/upgrade, search, install, completion and remove passed\n' "$backend"
