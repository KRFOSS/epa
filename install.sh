#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
set -eu
export LC_ALL=C
umask 077

fail() { printf 'epa 설치: %s\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = Linux ] || fail 'Linux에서만 설치할 수 있습니다.'
machine=$(uname -m)
case "$machine" in
    x86_64|amd64) arch=amd64 ;;
    i[3-6]86|x86) arch=386 ;;
    aarch64|arm64) arch=arm64 ;;
    armv8l|armv7l) arch=armv7 ;;
    armv6l) arch=armv6 ;;
    armv5tel|armv5tejl|armv5l) arch=armv5 ;;
    loongarch64|loong64) arch=loong64 ;;
    ppc64el|ppc64le) arch=ppc64le ;;
    ppc64|riscv64|s390x) arch=$machine ;;
    mips|mips64|mipsel|mipsle|mips64el|mips64le)
        # uname alone can omit the byte order, or report a 64-bit kernel
        # with a 32-bit userland. Read EI_CLASS and EI_DATA of this shell.
        elf=$(od -An -tu1 -j4 -N2 "/proc/$$/exe") || fail 'MIPS ABI를 확인할 수 없습니다.'
        set -- $elf
        case "$*" in
            '1 1') arch=mipsle ;;
            '1 2') arch=mips ;;
            '2 1') arch=mips64le ;;
            '2 2') arch=mips64 ;;
            *) fail '알 수 없는 MIPS ABI입니다.' ;;
        esac ;;
    *) fail "지원하지 않는 아키텍처: $machine" ;;
esac

for command in curl sha256sum awk mktemp install mv; do
    command -v "$command" >/dev/null 2>&1 || fail "$command 명령이 필요합니다."
done
if [ "$(id -u)" -eq 0 ]; then
    bin_dir=${EPA_BIN_DIR:-/usr/local/bin}
else
    bin_dir=${EPA_BIN_DIR:-${HOME:?HOME is unset}/.local/bin}
fi
case "$bin_dir" in /*) ;; *) fail 'EPA_BIN_DIR에는 절대 경로를 지정하세요.' ;; esac

work=$(mktemp -d) || fail '임시 디렉터리를 만들 수 없습니다.'
stage=
cleanup() {
    if [ -n "$stage" ]; then rm -f -- "$stage"; fi
    rm -rf -- "$work"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

base=https://github.com/KRFOSS/epa/releases/latest/download
asset=epa-linux-$arch
fetch() {
    curl -q --proto '=https' --proto-redir '=https' --fail --silent --show-error --location \
        --retry 3 --connect-timeout 15 --max-time 180 --output "$work/$1" "$base/$1"
}
fetch SHA256SUMS || fail '공개 릴리스의 체크섬 목록을 내려받지 못했습니다.'
fetch "$asset" || fail "릴리스에 $asset 파일이 없거나 내려받지 못했습니다."
expected=$(awk -v name="$asset" '$2 == name && NF == 2 {print $1; count++} END {if (count != 1) exit 1}' "$work/SHA256SUMS") || fail '대상 체크섬을 하나로 특정할 수 없습니다.'
case "$expected" in ''|*[!0-9a-f]*) fail '잘못된 체크섬 형식입니다.' ;; esac
[ "${#expected}" -eq 64 ] || fail '잘못된 체크섬 길이입니다.'
(cd "$work" && printf '%s  %s\n' "$expected" "$asset" | sha256sum -c - >/dev/null) || fail 'SHA-256 검증에 실패했습니다. 기존 파일은 변경하지 않았습니다.'

mkdir -p -- "$bin_dir" || fail "설치 경로를 만들 수 없습니다: $bin_dir"
stage=$(mktemp "$bin_dir/.epa.XXXXXX") || fail "설치 경로에 쓸 수 없습니다: $bin_dir"
install -m 0755 -- "$work/$asset" "$stage"
mv -fT -- "$stage" "$bin_dir/epa"
stage=
printf 'EPA 설치 완료: %s/epa\n' "$bin_dir"
# Register completion only for the account running the installer.
# A successful binary installation is not rolled back if a user's shell
# configuration cannot safely be modified.
if [ "${EPA_INSTALL_COMPLETION:-1}" != 0 ]; then
    if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ]; then
        printf 'sudo로 설치하여 자동완성 등록은 생략합니다. 사용할 계정에서 epa completion install을 실행하세요.\n'
    elif ! "$bin_dir/epa" completion install; then
        printf '자동완성 등록을 완료하지 못했습니다. 사용자 계정에서 epa completion install을 다시 실행하세요.\n' >&2
    fi
fi
case ":$PATH:" in
    *":$bin_dir:"*) ;;
    *) printf '이 경로를 PATH에 추가하거나 설치된 파일의 절대 경로로 실행하세요: %s\n' "$bin_dir" ;;
esac
