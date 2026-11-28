#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
set -eu
export LC_ALL=C
umask 077

fail() { printf 'epa 설치: %s\n' "$*" >&2; exit 1; }

[ "$#" -eq 2 ] || fail '사용법: sh install.sh vX.Y.Z /절대/경로/epa-release.der (신뢰할 수 있는 경로로 받은 공개키)'
tag=$1
key=$2
printf '%s\n' "$tag" | grep -Exq 'v(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})' || fail '정식 버전 태그(vX.Y.Z)를 지정하세요.'
# 패턴에 맞는 줄이 있더라도 여러 줄이면 거부합니다.
case "$tag" in *[!v0-9.]*) fail '정식 버전 태그(vX.Y.Z)를 지정하세요.' ;; esac
case "$key" in /*) ;; *) fail '공개키 파일은 절대 경로로 지정하세요.' ;; esac
[ -f "$key" ] && [ -r "$key" ] || fail '신뢰한 EPA 공개키 파일을 읽을 수 없습니다.'

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

for command in curl openssl sha256sum awk mktemp install mv od tr wc; do
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

base=https://github.com/KRFOSS/epa/releases/download/$tag
asset=epa-linux-$arch
fetch() {
    curl -q --proto '=https' --proto-redir '=https' --fail --silent --show-error --location \
        --retry 3 --connect-timeout 15 --max-time 180 --max-filesize "$2" \
        --output "$work/$1" "$base/$1" && [ "$(wc -c < "$work/$1")" -le "$2" ]
}
# 고정 길이의 Ed25519 SubjectPublicKeyInfo 공개키만 받습니다.
[ "$(wc -c < "$key")" -eq 44 ] && [ "$(od -An -v -tx1 -N12 "$key" | tr -d ' \n')" = 302a300506032b6570032100 ] || fail 'Ed25519 DER 공개키가 필요합니다.'
fetch SHA256SUMS.sig 64 || fail '릴리스 서명을 내려받지 못했습니다.'
fetch SHA256SUMS 1048576 || fail '서명된 체크섬 목록을 내려받지 못했습니다.'
[ "$(wc -c < "$work/SHA256SUMS.sig")" -eq 64 ] &&
    openssl pkeyutl -verify -pubin -keyform DER -inkey "$key" -rawin -in "$work/SHA256SUMS" -sigfile "$work/SHA256SUMS.sig" >/dev/null ||
    fail '릴리스 서명 검증에 실패했습니다. 기존 EPA는 변경하지 않았습니다.'
key_digest=$(sha256sum < "$key")
key_digest=${key_digest%% *}
{
    IFS= read -r header && [ "$header" = "# KRFOSS/epa release $tag" ] &&
    IFS= read -r source && printf '%s\n' "$source" | grep -Exq '# source [0-9a-f]{40}' &&
    IFS= read -r signed_key && [ "$signed_key" = "# key $key_digest" ]
} < "$work/SHA256SUMS" || fail '서명된 배포 목록의 버전·소스·공개키 정보가 일치하지 않습니다.'
expected=$(awk -v name="$asset" '$2 == name {if (NF != 2) exit 1; print $1; count++} END {if (count != 1) exit 1}' "$work/SHA256SUMS") || fail '대상 체크섬을 하나로 특정할 수 없습니다.'
case "$expected" in ''|*[!0-9a-f]*) fail '잘못된 체크섬 형식입니다.' ;; esac
[ "${#expected}" -eq 64 ] || fail '잘못된 체크섬 길이입니다.'
fetch "$asset" 67108864 || fail "릴리스에 $asset 파일이 없거나 내려받지 못했습니다."
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
