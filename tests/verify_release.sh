#!/usr/bin/env bash
# 정식 게시 전에 서명, 고정한 버전·소스·키, 배포 파일을 검증합니다.
set -euo pipefail
export LC_ALL=C
source "$(dirname -- "${BASH_SOURCE[0]}")/release_format.sh"
version=${1:?release tag required}
source_sha=${2:?source SHA required}
cd -- "${3:?distribution directory required}"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
release_public_key > "$work/key.der"
key_digest=$(sha256sum -- "$work/key.der")
key_digest=${key_digest%% *}
[[ -f SHA256SUMS.sig && $(wc -c < SHA256SUMS.sig) -eq 64 ]]
[[ -f SHA256SUMS && $(wc -c < SHA256SUMS) -le 1048576 ]]
if ! openssl pkeyutl -verify -pubin -keyform DER -inkey "$work/key.der" -rawin \
    -in SHA256SUMS -sigfile SHA256SUMS.sig; then
    echo '릴리스 서명 검증에 실패해 게시를 중단합니다.' >&2
    exit 1
fi
# 받은 목록의 경로를 해석하지 않고, 정해진 파일들의 목록을 재계산합니다.
shopt -s nullglob
files=(epa-linux-*)
[[ ${#files[@]} -eq ${#release_arches[@]} ]]
release_manifest "$version" "$source_sha" "$key_digest" > "$work/expected"
if ! cmp -s "$work/expected" SHA256SUMS; then
    echo '서명된 배포 목록이 요청한 버전·소스·공개키 또는 실제 배포 파일과 일치하지 않습니다.' >&2
    exit 1
fi
