#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-only
# 서명된 배포 파일로부터 독립 호스팅용 정적 파일만 생성합니다.
set -euo pipefail
export LC_ALL=C
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
version=${1:?정식 버전이 필요합니다.}
source_sha=${2:?소스 커밋 SHA가 필요합니다.}
dist=${3:?서명된 배포 파일 디렉터리가 필요합니다.}
out=${4:?새 출력 디렉터리가 필요합니다.}
base=https://krfoss.org/epa
[[ $version =~ ^v(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})$ && $source_sha =~ ^[0-9a-f]{40}$ ]]
bash "$root/tests/verify_release.sh" "$version" "$source_sha" "$dist"
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
# verify_release.sh가 공개키의 형식을 검사한 뒤에만 셸 소스에 삽입합니다.
helper=bootstrap-$version.sh
sed -e "s|@EPA_VERSION@|$version|" -e "s|@EPA_PUBLIC_KEY@|$EPA_RELEASE_PUBLIC_KEY|" \
    "$root/tools/install-bootstrap.sh" > "$work/$helper"
digest=$(sha256sum < "$work/$helper")
digest=${digest%% *}
printf '%s %s %s\n' 'sh -c '\''d=$(mktemp -d) || exit; cleanup(){ rm -rf -- "$d"; }; trap cleanup EXIT; (cd "$d" && curl -qfs --proto =https --max-time 60 --max-filesize 65536 -o boot "$1" && printf "%s  boot\n" "$2" | sha256sum -c - && sh ./boot)'\'' sh' "$base/$helper" "$digest" > "$work/install.txt"
{
    cat <<HTML
<!doctype html>
<html lang="ko">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>EPA 설치 · ROKFOSS</title>
<style>html{color-scheme:light dark}body{font:1rem/1.6 system-ui,sans-serif;max-width:55rem;margin:3rem auto;padding:0 1.25rem}pre{overflow:auto;padding:1rem;border:1px solid GrayText}button{font:inherit;padding:.35rem .7rem}</style>
<main>
<h1>EPA 설치</h1>
<p>Linux에서 EPA $version을 설치합니다. 아래 한 줄을 복사해 터미널에서 실행하세요.</p>
<pre><code id="command">
HTML
    sed 's/\&/\&amp;/g; s/</\&lt;/g; s/>/\&gt;/g' "$work/install.txt"
    cat <<'HTML'
</code></pre>
<button id="copy" hidden type="button">명령 복사</button> <span id="status" role="status"></span>
<script>
const button = document.getElementById('copy');
if (navigator.clipboard) {
    button.hidden = false;
    button.addEventListener('click', async () => {
        const status = document.getElementById('status');
        try {
            await navigator.clipboard.writeText(document.getElementById('command').textContent.trim());
            status.textContent = '복사했습니다.';
        } catch {
            status.textContent = '명령을 선택해 직접 복사하세요.';
        }
    });
}
</script>
<p><code>curl</code>, <code>sha256sum</code>, OpenSSL 3.x가 필요합니다. 서명 검증에 실패하면 기존 EPA는 변경되지 않습니다.</p>
</main>
</html>
HTML
} > "$work/index.html"
# 이전에 만든 페이지나 도우미를 덮어쓰지 않습니다.
mkdir -- "$out"
install -m 0644 -- "$work/$helper" "$work/install.txt" "$work/index.html" "$out/"
printf '설치 페이지 파일 준비 완료: %s\n' "$out"
