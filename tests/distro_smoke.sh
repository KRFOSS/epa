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
 apt|dnf)
    if [[ "$backend" == apt ]]; then
        dpkg-query -W > "$XDG_CONFIG_HOME/native.before"
    else
        rpm -qa | sort > "$XDG_CONFIG_HOME/native.before"
    fi
    run update 2> "$XDG_CONFIG_HOME/refresh.log"
    cat "$XDG_CONFIG_HOME/refresh.log"
    if [[ "$backend" == apt ]]; then
        apt -o APT::Cmd::Disable-Script-Warning=true -o APT::Color=false list --upgradable > "$XDG_CONFIG_HOME/candidates"
        count=$(awk '/\[upgradable from: / {print $1, $3}' "$XDG_CONFIG_HOME/candidates" | sort -u | wc -l)
        dpkg-query -W > "$XDG_CONFIG_HOME/native.after"
    else
        dnf -q --cacheonly repoquery --upgrades --queryformat='%{name}.%{arch}' > "$XDG_CONFIG_HOME/candidates"
        count=$(sort -u "$XDG_CONFIG_HOME/candidates" | sed '/^[[:space:]]*$/d' | wc -l)
        rpm -qa | sort > "$XDG_CONFIG_HOME/native.after"
    fi
    if [[ "$count" == 0 ]]; then
        grep -F '시스템 패키지: 갱신한 저장소 기준 업그레이드 후보가 없습니다.' "$XDG_CONFIG_HOME/refresh.log"
    else
        grep -F "시스템 패키지: 업그레이드 후보 ${count}개." "$XDG_CONFIG_HOME/refresh.log"
    fi
    test "$(sha256sum < "$XDG_CONFIG_HOME/native.before")" = "$(sha256sum < "$XDG_CONFIG_HOME/native.after")"
    "$bin" --style pacman --progress native -Syy
    ;;
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
    run update 2> "$XDG_CONFIG_HOME/refresh.log"
    cat "$XDG_CONFIG_HOME/refresh.log"
    test -f /var/cache/epa/pacman/.epa-ready
    query_rc=0
    pacman --dbpath /var/cache/epa/pacman --color never -Quq > "$XDG_CONFIG_HOME/candidates" 2> "$XDG_CONFIG_HOME/query.err" || query_rc=$?
    test ! -s "$XDG_CONFIG_HOME/query.err"
    count=$(wc -l < "$XDG_CONFIG_HOME/candidates")
    if [[ "$count" == 0 ]]; then
        test "$query_rc" -eq 1 || test "$query_rc" -eq 0
        grep -F '시스템 패키지: 갱신한 저장소 기준 업그레이드 후보가 없습니다.' "$XDG_CONFIG_HOME/refresh.log"
    else
        test "$query_rc" -eq 0
        grep -F "시스템 패키지: 업그레이드 후보 ${count}개." "$XDG_CONFIG_HOME/refresh.log"
        grep -F 'epa --style apt list --upgradable' "$XDG_CONFIG_HOME/refresh.log"
    fi
    native_digest > "$XDG_CONFIG_HOME/native.after"
    test "$(sha256sum < "$XDG_CONFIG_HOME/native.before")" = "$(sha256sum < "$XDG_CONFIG_HOME/native.after")"
    run --dry-run search tree | grep -F '/var/cache/epa/pacman'
    run search tree | grep -F tree
    "$bin" __complete -- install tre | grep -Fx tree
    "$bin" --style pacman --progress native -Syy
    native_digest > "$XDG_CONFIG_HOME/native.after"
    test "$(sha256sum < "$XDG_CONFIG_HOME/native.before")" = "$(sha256sum < "$XDG_CONFIG_HOME/native.after")"
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
# Resolve patterns against the native installation database, not the
# isolated Arch query snapshot. The dry run must contain only exact names.
run --dry-run install "$package*" -y > "$XDG_CONFIG_HOME/pattern.plan"
grep -F "\"$package\"" "$XDG_CONFIG_HOME/pattern.plan"
if grep -F "\"$package*\"" "$XDG_CONFIG_HOME/pattern.plan"; then exit 1; fi
if run --dry-run install 'epa-no-such-package-917294*'; then exit 1; fi
if run --dry-run remove "$package*"; then exit 1; fi
printf 'Testing native %s transaction with %s\n' "$backend" "$package"
run install "$package" -y
command -v "$package"
"$bin" __complete -- remove "${package:0:2}" | grep -Fx -- "$package"
run remove "$package" -y
if command -v "$package" >/dev/null; then
    printf '%s is still installed\n' "$package" >&2
    exit 1
fi
if [[ "$backend" == pacman ]]; then
    pacman -Q > "$XDG_CONFIG_HOME/clean.before"
    run --dry-run clean -y | grep -F '"-Sc"'
    run clean -y
    pacman -Q > "$XDG_CONFIG_HOME/clean.after"
    test "$(sha256sum < "$XDG_CONFIG_HOME/clean.before")" = "$(sha256sum < "$XDG_CONFIG_HOME/clean.after")"
fi
printf 'Native %s refresh/upgrade, search, install, completion and remove passed\n' "$backend"
