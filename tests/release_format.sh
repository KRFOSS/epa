#!/usr/bin/env bash
# 빌드와 게시 검증이 같은 배포 파일 목록과 형식을 사용합니다.
release_arches=(386 amd64 armv5 armv6 armv7 arm64 loong64 mips mipsle mips64 mips64le ppc64 ppc64le riscv64 s390x)

release_public_key() {
    local encoded=${EPA_RELEASE_PUBLIC_KEY:-}
    # 44바이트 Ed25519 SPKI DER의 Base64입니다. GitHub 변수에는 공개키만 둡니다.
    if [[ ! $encoded =~ ^MCowBQYDK2VwAyEA[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]=$ ]]; then
        echo 'EPA_RELEASE_PUBLIC_KEY에 올바른 Ed25519 DER 공개키의 Base64가 필요합니다.' >&2
        return 1
    fi
    printf '%s' "$encoded" | base64 --decode
}

release_manifest() {
    local tag=$1 source_sha=$2 key_digest=$3 arch
    local assets=()
    for arch in "${release_arches[@]}"; do
        assets+=("epa-linux-$arch")
    done
    printf '# KRFOSS/epa release %s\n# source %s\n# key %s\n' "$tag" "$source_sha" "$key_digest"
    sha256sum -- "${assets[@]}" install.sh
}
