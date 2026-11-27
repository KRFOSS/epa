#!/usr/bin/env bash
# Run only as root inside a disposable Linux container.
set -euo pipefail
export LC_ALL=C
export XDG_CONFIG_HOME
XDG_CONFIG_HOME=$(mktemp -d)
trap 'rm -rf -- "$XDG_CONFIG_HOME"' EXIT
unset SUDO_USER SUDO_COMMAND
bin=${1:?EPA binary required}
backend=${2:?expected backend required}
run() { "$bin" --style apt --progress native "$@"; }
"$bin" backend info | grep -F "백엔드: $backend ("
# Inherited sudo variables in a root login shell must not block root
# preferences or redirect writes into the original user's configuration.
SUDO_USER=operator SUDO_COMMAND=/bin/bash "$bin" style set pacman
SUDO_USER=operator SUDO_COMMAND=/bin/bash "$bin" style current | grep -Fx pacman
SUDO_USER=operator SUDO_COMMAND=/usr/bin/fish "$bin" config set language en
SUDO_USER=operator SUDO_COMMAND=/usr/bin/fish "$bin" config get language | grep -Fx en
SUDO_USER=operator SUDO_COMMAND=/usr/bin/fish "$bin" config set language ko
SUDO_USER=operator SUDO_COMMAND=/bin/bash "$bin" style set apt
if SUDO_USER=operator SUDO_COMMAND="$bin style set dnf" "$bin" style set dnf; then
    echo 'direct sudo unexpectedly modified the account style' >&2
    exit 1
fi
test ! -e "$XDG_CONFIG_HOME/epa/config.json"
case "$backend" in
 apt|dnf) run update; "$bin" --style pacman --progress native -Syy ;;
 pacman)
    native_db=$(pacman-conf DBPath)
    native_digest() {
        if [[ -d "$native_db/sync" ]]; then
            find "$native_db/sync" -type f -print0 | sort -z | xargs -0 -r sha256sum
        fi
        pacman -Q
    }
    native_digest > "$XDG_CONFIG_HOME/native.before"
    run --dry-run update | grep -F '/var/cache/epa/pacman'
    test ! -e /var/cache/epa/pacman/.epa-ready
    run update
    test -f /var/cache/epa/pacman/.epa-ready
    native_digest > "$XDG_CONFIG_HOME/native.after"
    cmp "$XDG_CONFIG_HOME/native.before" "$XDG_CONFIG_HOME/native.after"
    run --dry-run search tree | grep -F '/var/cache/epa/pacman'
    run search tree | grep -F tree
    "$bin" __complete -- install tre | grep -Fx tree
    "$bin" --style pacman --progress native -Syy
    native_digest > "$XDG_CONFIG_HOME/native.after"
    cmp "$XDG_CONFIG_HOME/native.before" "$XDG_CONFIG_HOME/native.after"
    run --dry-run install tree -y > "$XDG_CONFIG_HOME/install.plan"
    if grep -F '/var/cache/epa/pacman' "$XDG_CONFIG_HOME/install.plan"; then exit 1; fi
    run --dry-run upgrade -y | grep -F '"-Syu"'
    run upgrade -y
    ;;
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
