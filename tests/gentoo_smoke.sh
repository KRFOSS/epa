#!/bin/bash
# Run only in the official disposable Gentoo stage3 container, with an
# official Portage repository snapshot mounted at /var/db/repos/gentoo.
set -euo pipefail
export LC_ALL=C
export XDG_CONFIG_HOME
XDG_CONFIG_HOME=$(mktemp -d)
trap 'rm -rf -- "$XDG_CONFIG_HOME"' EXIT
unset SUDO_USER SUDO_COMMAND
bin=${1:?EPA binary required}
run() { "$bin" --style apt --progress native "$@"; }
emerge --version
"$bin" backend info | grep -F '백엔드: portage ('
test -f /var/db/repos/gentoo/profiles/repo_name

# A tiny local overlay is the only repository we synchronize. No real
# Gentoo repository or installed package is modified outside this container.
src=/tmp/epa-smoke-source
repo=/var/db/repos/epa-smoke
mkdir -p "$src/profiles" "$src/metadata" "$repo/profiles" "$repo/metadata"
printf '%s\n' epa-smoke > "$src/profiles/repo_name"
printf '%s\n' app-misc > "$src/profiles/categories"
printf '%s\n' 'masters = gentoo' 'thin-manifests = true' > "$src/metadata/layout.conf"
date -u '+%a, %d %b %Y %H:%M:%S +0000' > "$src/metadata/timestamp.chk"
cp "$src/profiles/repo_name" "$src/profiles/categories" "$repo/profiles/"
cp "$src/metadata/layout.conf" "$repo/metadata/"
if test -e /etc/portage/repos.conf; then
    mv /etc/portage/repos.conf /etc/portage/repos.conf.epa-backup
fi
cat > /etc/portage/repos.conf <<EOF
[DEFAULT]
main-repo = gentoo
[gentoo]
location = /var/db/repos/gentoo
auto-sync = no
[epa-smoke]
location = $repo
sync-type = rsync
sync-uri = file://$src
auto-sync = yes
EOF

# No remote binhost is required for the fixture's source-only ebuilds.
export EMERGE_DEFAULT_OPTS='--getbinpkg=n --usepkg=n'
for pkg in epa-smoke-dependency epa-smoke-test; do
    mkdir -p "$src/app-misc/$pkg"
    cat > "$src/app-misc/$pkg/$pkg-1.ebuild" <<EOF
EAPI=8
DESCRIPTION="EPA disposable native transaction fixture"
HOMEPAGE="https://github.com/KRFOSS/epa"
SLOT="0"
LICENSE="MIT"
KEYWORDS="amd64"
S="\${WORKDIR}"
src_install() {
    dodir /usr/share/$pkg
    printf '%s\\n' '$pkg' > "\${ED}/usr/share/$pkg/marker"
}
EOF
done
printf '%s\n' 'RDEPEND="app-misc/epa-smoke-dependency"' >> "$src/app-misc/epa-smoke-test/epa-smoke-test-1.ebuild"
run --dry-run update | grep -F '"--sync"'
run update 2> "$XDG_CONFIG_HOME/refresh.log"
cat "$XDG_CONFIG_HOME/refresh.log"
# The stage3 snapshot may have unsatisfiable @world dependencies.
# Never turn a failed solver query into an empty upgrade list.
grep -E '시스템 패키지: @world 갱신 계획 [0-9]+개|시스템 패키지 업데이트 후보를 확인하지 못했습니다' "$XDG_CONFIG_HOME/refresh.log"
test -f "$repo/app-misc/epa-smoke-test/epa-smoke-test-1.ebuild"
for pkg in epa-smoke-dependency epa-smoke-test; do
    ebuild "$repo/app-misc/$pkg/$pkg-1.ebuild" manifest
done

run search epa-smoke-test | grep -F app-misc/epa-smoke-test
run show app-misc/epa-smoke-test | grep -F 'EPA disposable native transaction fixture'
run list | grep -F sys-apps/portage-
"$bin" __complete -- install app-misc/epa-smoke-te | grep -Fx app-misc/epa-smoke-test
"$bin" style set emerge
"$bin" style current | grep -Fx emerge
"$bin" __complete -- -avuDN @ | grep -Fx @world
"$bin" style reset
run --dry-run upgrade | grep -F '"@world"'
run --dry-run outdated | grep -F '"--pretend"'
run --dry-run autoremove | grep -F '"--depclean"'
"$bin" --style pacman --progress native --dry-run -Syu | grep -F '"--sync"'
if run install app-misc/epa-smoke-test --nodeps; then exit 1; fi
if run install app-misc/epa-smoke-test.ebuild; then exit 1; fi
if "$bin" --style emerge --progress native --unmerge app-misc/epa-smoke-test; then exit 1; fi

# The real merge prompt must be forwarded, and declining it must leave
# both fixture packages uninstalled.
python3 /epa/tests/gentoo_decline.py "$bin" --style apt --progress native install app-misc/epa-smoke-test
test -z "$(portageq match / app-misc/epa-smoke-test || :)"
test -z "$(portageq match / app-misc/epa-smoke-dependency || :)"

"$bin" --style emerge --progress plain --ask=n app-misc/epa-smoke-test
test -f /usr/share/epa-smoke-test/marker
test -f /usr/share/epa-smoke-dependency/marker
portageq match / app-misc/epa-smoke-test | grep -Fx app-misc/epa-smoke-test-1
"$bin" __complete -- remove app-misc/epa-smoke-te | grep -Fx app-misc/epa-smoke-test
world_before=$(sha256sum /var/lib/portage/world)
run reinstall app-misc/epa-smoke-test -y
test -f /usr/share/epa-smoke-test/marker
test "$world_before" = "$(sha256sum /var/lib/portage/world)"

# A targeted removal may fail or retain a package still required by another.
run remove app-misc/epa-smoke-dependency -y || :
test -f /usr/share/epa-smoke-dependency/marker
test -f /usr/share/epa-smoke-test/marker
run remove app-misc/epa-smoke-test -y
test -z "$(portageq match / app-misc/epa-smoke-test || :)"
if test -n "$(portageq match / app-misc/epa-smoke-dependency || :)"; then
    run remove app-misc/epa-smoke-dependency -y
fi
test -z "$(portageq match / app-misc/epa-smoke-dependency || :)"
printf '%s\n' 'Gentoo local sync, query, completion, confirmation, install, rebuild and dependency-safe removal passed'
