#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-only
set -eu
export LC_ALL=C
umask 077

tag='@EPA_VERSION@'
public_key='@EPA_PUBLIC_KEY@'
fail() { printf 'epa 설치: %s\n' "$*" >&2; exit 1; }
[ "$#" -eq 0 ] || fail '설치 명령에는 인수가 필요하지 않습니다.'
for tool in curl openssl sha256sum awk grep mktemp base64 wc; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool 명령이 필요합니다."
done
work=$(mktemp -d) || fail '임시 디렉터리를 만들 수 없습니다.'
trap 'rm -rf -- "$work"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
case "$work" in /*) ;; *) work=$PWD/$work ;; esac
key=$work/epa-release.der
printf '%s' "$public_key" | base64 -d > "$key" || fail '내장 공개키를 읽을 수 없습니다.'
key_hash=$(sha256sum < "$key")
key_hash=${key_hash%% *}
base=https://github.com/KRFOSS/epa/releases/download/$tag
fetch() {
    curl -qfsSL --proto '=https' --proto-redir '=https' --connect-timeout 15 --max-time 60 \
        --max-filesize "$2" -o "$work/$1" "$base/$1" && [ "$(wc -c < "$work/$1")" -le "$2" ]
}
fetch SHA256SUMS.sig 64 || fail '공개 릴리스 서명을 내려받지 못했습니다.'
fetch SHA256SUMS 1048576 || fail '서명된 배포 목록을 내려받지 못했습니다.'
[ "$(wc -c < "$work/SHA256SUMS.sig")" -eq 64 ] &&
    openssl pkeyutl -verify -pubin -keyform DER -inkey "$key" -rawin -in "$work/SHA256SUMS" \
        -sigfile "$work/SHA256SUMS.sig" >/dev/null || fail '릴리스 서명이 올바르지 않습니다. 설치하지 않았습니다.'
{
    IFS= read -r header && [ "$header" = "# KRFOSS/epa release $tag" ] &&
    IFS= read -r source && printf '%s\n' "$source" | grep -Exq '# source [0-9a-f]{40}' &&
    IFS= read -r signed_key && [ "$signed_key" = "# key $key_hash" ]
} < "$work/SHA256SUMS" || fail '서명된 배포 목록이 요청한 버전 또는 공개키와 일치하지 않습니다.'
expected=$(awk '$2 == "install.sh" { if (NF != 2) exit 1; print $1; n++ } END { if (n != 1) exit 1 }' "$work/SHA256SUMS") ||
    fail '설치기 체크섬을 하나로 특정할 수 없습니다.'
case "$expected" in ''|*[!0-9a-f]*) fail '설치기 체크섬 형식이 올바르지 않습니다.' ;; esac
[ "${#expected}" -eq 64 ] || fail '설치기 체크섬 길이가 올바르지 않습니다.'
fetch install.sh 1048576 || fail '설치기를 내려받지 못했습니다.'
(cd "$work" && printf '%s  install.sh\n' "$expected" | sha256sum -c - >/dev/null) ||
    fail '설치기 SHA-256이 일치하지 않습니다. 설치하지 않았습니다.'
sh "$work/install.sh" "$tag" "$key"
