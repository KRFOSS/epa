#!/usr/bin/env bash
# Uses real release artifacts with an offline replacement for curl.
set -euo pipefail
export LC_ALL=C
export ASSETS
ASSETS=$(cd -- "${1:?release artifacts required}" && pwd)
version=${2:?expected version required}
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/shims"
cat > "$work/shims/curl" <<'SH'
#!/bin/sh
while [ "$1" != --output ]; do shift; done
name=${3##*/}
case "$3" in https://github.com/KRFOSS/epa/releases/latest/download/*) ;; *) exit 50 ;; esac
[ "$name" = SHA256SUMS ] || [ "$name" = "epa-linux-$EXPECT_ARCH" ] || exit 51
if [ "$name" != SHA256SUMS ] && [ "${FETCH_MODE:-}" = missing ]; then exit 22; fi
cp "$ASSETS/$name" "$2"
if [ "$name" != SHA256SUMS ] && [ "${FETCH_MODE:-}" = corrupt ]; then echo corrupt >> "$2"; fi
SH
cat > "$work/shims/uname" <<'SH'
#!/bin/sh
case "$1" in -s) echo "${TEST_OS:-Linux}" ;; -m) echo "$TEST_MACHINE" ;; *) exit 1 ;; esac
SH
cat > "$work/shims/od" <<'SH'
#!/bin/sh
printf '%s\n' "$TEST_ELF"
SH
cat > "$work/shims/id" <<'SH'
#!/bin/sh
[ "$1" = '-u' ] && echo 1001
SH
chmod +x "$work/shims/"*
export PATH="$work/shims:$PATH" HOME="$work/home"
export TEST_MACHINE EXPECT_ARCH TEST_ELF
unset EPA_BIN_DIR ZDOTDIR SUDO_USER SUDO_COMMAND
# Cross-architecture cases validate bytes; foreign binaries cannot run here.
export EPA_INSTALL_COMPLETION=0
export XDG_CONFIG_HOME="$HOME/.config"
for spec in 'x86_64 amd64' 'i686 386' 'armv5tel armv5' 'armv6l armv6' 'armv7l armv7' 'armv8l armv7' 'aarch64 arm64' 'loongarch64 loong64' 'mips mips 1 2' 'mips mipsle 1 1' 'mips64 mips64 2 2' 'mips64 mips64le 2 1' 'ppc64 ppc64' 'ppc64le ppc64le' 'riscv64 riscv64' 's390x s390x'; do
    read -r TEST_MACHINE EXPECT_ARCH TEST_ELF <<< "$spec"
    sh "$ASSETS/install.sh" > "$work/log"
    cmp "$ASSETS/epa-linux-$EXPECT_ARCH" "$HOME/.local/bin/epa"
    test -x "$HOME/.local/bin/epa"
done
TEST_MACHINE=x86_64 EXPECT_ARCH=amd64
export EPA_BIN_DIR="$work/bin with spaces"
sh "$ASSETS/install.sh"
[[ $("$EPA_BIN_DIR/epa" --version) == "epa $version" ]]
test ! -e "$HOME/.bashrc"
test ! -e "$HOME/.zshrc"
test ! -e "$XDG_CONFIG_HOME/fish/completions/epa.fish"
unset EPA_INSTALL_COMPLETION
sh "$ASSETS/install.sh"
grep -Fx '# >>> EPA shell completion >>>' "$HOME/.bashrc"
grep -Fx '# >>> EPA shell completion >>>' "$HOME/.profile"
if command -v zsh >/dev/null 2>&1; then
    grep -Fx '# >>> EPA shell completion >>>' "$HOME/.zshrc"
fi
if command -v fish >/dev/null 2>&1; then
    grep -F "command epa completion fish | source" "$XDG_CONFIG_HOME/fish/completions/epa.fish"
fi
cp "$HOME/.bashrc" "$work/bashrc.before"
sh "$ASSETS/install.sh"
cmp "$HOME/.bashrc" "$work/bashrc.before"
for mode in corrupt missing; do
    if FETCH_MODE=$mode sh "$ASSETS/install.sh" > "$work/log" 2>&1; then
        echo "installer unexpectedly accepted $mode payload" >&2
        exit 1
    fi
    cmp "$ASSETS/epa-linux-amd64" "$EPA_BIN_DIR/epa"
done
grep -q '내려받지 못했습니다' "$work/log"
if TEST_MACHINE=sparc64 sh "$ASSETS/install.sh" > "$work/log" 2>&1; then exit 1; fi
grep -q '지원하지 않는 아키텍처' "$work/log"
if TEST_OS=Darwin sh "$ASSETS/install.sh" > "$work/log" 2>&1; then exit 1; fi
grep -q 'Linux에서만' "$work/log"
echo '설치 테스트 통과'
