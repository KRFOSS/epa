#!/usr/bin/env bash
# Uses real release artifacts with an offline replacement for curl.
set -euo pipefail
export LC_ALL=C
export ASSETS
ASSETS=$(cd -- "${1:?release artifacts required}" && pwd)
version=${2:?expected version required}
tag=v$version
key=${3:?trusted test public key required}
export TEST_TAG=$tag
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/shims"
cat > "$work/shims/curl" <<'SH'
#!/bin/sh
while [ "$1" != --output ] && [ "$1" != -o ]; do shift; done
name=${3##*/}
case "$3" in "https://github.com/KRFOSS/epa/releases/download/$TEST_TAG/"*) ;; *) exit 50 ;; esac
case "$name" in SHA256SUMS|SHA256SUMS.sig|install.sh|"epa-linux-$EXPECT_ARCH") ;; *) exit 51 ;; esac
printf '%s\n' "$name" >> "$FETCH_LOG"
case "${FETCH_MODE:-}:$name" in
    missing:epa-linux-*|missing-signature:SHA256SUMS.sig) exit 22 ;;
esac
cp "$ASSETS/$name" "$2"
case "${FETCH_MODE:-}:$name" in
    corrupt:epa-linux-*|corrupt-manifest:SHA256SUMS|corrupt-installer:install.sh|oversized-signature:SHA256SUMS.sig) echo corrupt >> "$2" ;;
    short-signature:SHA256SUMS.sig) printf 'test\n' > "$2" ;;
esac
SH
cat > "$work/shims/uname" <<'SH'
#!/bin/sh
case "$1" in -s) echo "${TEST_OS:-Linux}" ;; -m) echo "$TEST_MACHINE" ;; *) exit 1 ;; esac
SH
cat > "$work/shims/od" <<'SH'
#!/bin/sh
case "$*" in '-An -tu1 -j4 -N2 '*) printf '%s\n' "$TEST_ELF" ;; *) exec /usr/bin/od "$@" ;; esac
SH
cat > "$work/shims/id" <<'SH'
#!/bin/sh
[ "$1" = '-u' ] && echo 1001
SH
chmod +x "$work/shims/"*
export PATH="$work/shims:$PATH" HOME="$work/home"
export TEST_MACHINE EXPECT_ARCH TEST_ELF FETCH_LOG="$work/requests"
unset EPA_BIN_DIR ZDOTDIR SUDO_USER SUDO_COMMAND
# Cross-architecture cases validate bytes; foreign binaries cannot run here.
export EPA_INSTALL_COMPLETION=0
export XDG_CONFIG_HOME="$HOME/.config"
for spec in 'x86_64 amd64' 'i686 386' 'armv5tel armv5' 'armv6l armv6' 'armv7l armv7' 'armv8l armv7' 'aarch64 arm64' 'loongarch64 loong64' 'mips mips 1 2' 'mips mipsle 1 1' 'mips64 mips64 2 2' 'mips64 mips64le 2 1' 'ppc64 ppc64' 'ppc64le ppc64le' 'riscv64 riscv64' 's390x s390x'; do
    read -r TEST_MACHINE EXPECT_ARCH TEST_ELF <<< "$spec"
    sh "$ASSETS/install.sh" "$tag" "$key" > "$work/log"
    cmp "$ASSETS/epa-linux-$EXPECT_ARCH" "$HOME/.local/bin/epa"
    test -x "$HOME/.local/bin/epa"
done
TEST_MACHINE=x86_64 EXPECT_ARCH=amd64
export EPA_BIN_DIR="$work/bin with spaces"
sh "$ASSETS/install.sh" "$tag" "$key"
[[ $("$EPA_BIN_DIR/epa" --version) == "epa $version" ]]
test ! -e "$HOME/.bashrc"
test ! -e "$HOME/.zshrc"
test ! -e "$XDG_CONFIG_HOME/fish/completions/epa.fish"
unset EPA_INSTALL_COMPLETION
sh "$ASSETS/install.sh" "$tag" "$key"
grep -Fx '# >>> EPA shell completion >>>' "$HOME/.bashrc"
grep -Fx '# >>> EPA shell completion >>>' "$HOME/.profile"
if command -v zsh >/dev/null 2>&1; then
    grep -Fx '# >>> EPA shell completion >>>' "$HOME/.zshrc"
fi
if command -v fish >/dev/null 2>&1; then
    grep -F "command epa completion fish | source" "$XDG_CONFIG_HOME/fish/completions/epa.fish"
fi
cp "$HOME/.bashrc" "$work/bashrc.before"
sh "$ASSETS/install.sh" "$tag" "$key"
cmp "$HOME/.bashrc" "$work/bashrc.before"
for mode in corrupt missing; do
    if FETCH_MODE=$mode sh "$ASSETS/install.sh" "$tag" "$key" > "$work/log" 2>&1; then
        echo "installer unexpectedly accepted $mode payload" >&2
        exit 1
    fi
    cmp "$ASSETS/epa-linux-amd64" "$EPA_BIN_DIR/epa"
done
grep -q '내려받지 못했습니다' "$work/log"

# 목록 또는 서명을 신뢰할 수 없으면 기존 파일을 보존하고 바이너리 요청도 하지 않습니다.
check_blocked() {
    : > "$FETCH_LOG"
    if "$@" > "$work/log" 2>&1; then
        echo '신뢰할 수 없는 설치를 차단하지 못했습니다.' >&2
        cat "$work/log" >&2
        exit 1
    fi
    if grep -q '^epa-linux-' "$FETCH_LOG"; then
        echo '검증에 실패했는데 바이너리를 내려받았습니다.' >&2
        exit 1
    fi
    cmp "$ASSETS/epa-linux-amd64" "$EPA_BIN_DIR/epa"
}
for mode in corrupt-manifest missing-signature short-signature oversized-signature; do
    check_blocked env FETCH_MODE="$mode" sh "$ASSETS/install.sh" "$tag" "$key"
done
openssl genpkey -algorithm ED25519 | openssl pkey -pubout -outform DER -out "$work/foreign.der"
check_blocked sh "$ASSETS/install.sh" "$tag" "$work/foreign.der"
check_blocked sh "$ASSETS/install.sh"
check_blocked env TEST_TAG=v999.1.0 sh "$ASSETS/install.sh" v999.1.0 "$key"

# README의 첫 설치 명령도 검증하고, 바꿔치기된 설치기는 실행하지 않습니다.
awk '/^```sh$/ { if (copy) exit; copy=1; next } /^```$/ && copy { exit } copy { print }' README.md |
    sed -e "s|^    tag=.*|    tag='$tag'|" -e "s|^    key=.*|    key='$key'|" > "$work/bootstrap.sh"
sh "$work/bootstrap.sh" > "$work/log"
grep -q 'EPA 설치 완료' "$work/log"
check_blocked env FETCH_MODE=corrupt-installer sh "$work/bootstrap.sh"
if TEST_MACHINE=sparc64 sh "$ASSETS/install.sh" "$tag" "$key" > "$work/log" 2>&1; then exit 1; fi
grep -q '지원하지 않는 아키텍처' "$work/log"
if TEST_OS=Darwin sh "$ASSETS/install.sh" "$tag" "$key" > "$work/log" 2>&1; then exit 1; fi
grep -q 'Linux에서만' "$work/log"
echo '설치 테스트 통과'
