# EPA 수동 서명 검증 및 설치

서명된 정식 릴리스부터 아래 설치 절차를 지원합니다. OpenSSL 3.x, `curl`, `sha256sum`이 필요합니다. **먼저 GitHub 릴리스와 독립적으로 신뢰할 수 있는 ROKFOSS 경로에서 공식 Ed25519 DER 공개키와 SHA-256 지문을 확인하세요.** 같은 릴리스에서 내려받은 키를 그대로 신뢰하거나, 서명되지 않은 기존 설치기를 대신 실행하지 마세요.

아래의 `tag`는 설치할 정식 버전으로, `key`는 지문을 확인한 공개키 파일의 절대 경로로 바꿔 실행하세요. 설치 스크립트도 검증하기 전에는 실행하지 않습니다.

```sh
(
    set -eu
    export LC_ALL=C
    umask 077
    tag='vX.Y.Z'
    key='/절대/경로/epa-release.der'
    case "$tag" in *[!v0-9.]*) exit 1 ;; esac
    printf '%s\n' "$tag" | grep -Exq 'v(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})'
    case "$key" in /*) ;; *) exit 1 ;; esac
    dir=$(mktemp -d)
    trap 'rm -rf -- "$dir"' EXIT
    cd "$dir"
    for file in SHA256SUMS.sig SHA256SUMS install.sh; do
        curl -qfsSL --proto '=https' --proto-redir '=https' --max-time 60 --max-filesize 1048576 \
            -o "$file" "https://github.com/KRFOSS/epa/releases/download/$tag/$file"
        test "$(wc -c < "$file")" -le 1048576
    done
    test "$(wc -c < "$key")" -eq 44
    test "$(od -An -v -tx1 -N12 "$key" | tr -d ' \n')" = 302a300506032b6570032100
    test "$(wc -c < SHA256SUMS.sig)" -eq 64
    openssl pkeyutl -verify -pubin -keyform DER -inkey "$key" -rawin -in SHA256SUMS -sigfile SHA256SUMS.sig
    IFS= read -r header < SHA256SUMS
    test "$header" = "# KRFOSS/epa release $tag"
    awk '$2 == "install.sh" { if (NF != 2) exit 1; print $1 "  install.sh"; n++ } END { if (n != 1) exit 1 }' SHA256SUMS > installer.sha256
    sha256sum -c installer.sha256
    sh ./install.sh "$tag" "$key"
)
```

검증에 실패하면 설치하지 마세요. 일반 사용자는 `~/.local/bin/epa`, root는 `/usr/local/bin/epa`에 설치됩니다.
