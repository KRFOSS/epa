#!/usr/bin/env bash
# 격리된 테스트 키로 서명하며 실제 태그·릴리스·서버를 변경하지 않습니다.
set -euo pipefail
export LC_ALL=C
umask 077
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
version=v0.0.0
source_sha=$(git rev-parse HEAD)
dist=$work/dist
if EPA_RELEASE_PUBLIC_KEY= bash tests/build_release.sh "$version" "$work/no-key" "$source_sha"; then
    echo '공개키 없이 배포 빌드를 허용했습니다.' >&2
    exit 1
fi
test ! -e "$work/no-key"
openssl genpkey -algorithm ED25519 -out "$work/key.pem"
openssl pkey -in "$work/key.pem" -pubout -outform DER -out "$work/key.der"
export EPA_RELEASE_PUBLIC_KEY
EPA_RELEASE_PUBLIC_KEY=$(base64 -w0 "$work/key.der")
bash tests/build_release.sh "$version" "$dist" "$source_sha"
# 빌드 결과의 실행 파일들이 모두 같은 공개키를 내장했는지 확인합니다.
for binary in "$dist"/epa-linux-*; do
    grep -aFq -- "$EPA_RELEASE_PUBLIC_KEY" "$binary"
done
if bash tests/verify_release.sh "$version" "$source_sha" "$dist"; then
    echo '서명 없는 배포 파일이 게시 검증을 통과했습니다.' >&2
    exit 1
fi
openssl pkeyutl -sign -inkey "$work/key.pem" -rawin -in "$dist/SHA256SUMS" -out "$dist/SHA256SUMS.sig"
rm -- "$work/key.pem"
bash tests/verify_release.sh "$version" "$source_sha" "$dist"
if bash tests/verify_release.sh v0.0.1 "$source_sha" "$dist"; then
    echo '다른 버전의 서명된 배포 파일이 게시 검증을 통과했습니다.' >&2
    exit 1
fi
if bash tests/verify_release.sh "$version" ffffffffffffffffffffffffffffffffffffffff "$dist"; then
    echo '다른 소스 커밋의 배포 파일이 게시 검증을 통과했습니다.' >&2
    exit 1
fi
mv -- "$dist/install.sh" "$work/install.sh"
printf '# 설치기 변경 테스트\n' > "$dist/install.sh"
if bash tests/verify_release.sh "$version" "$source_sha" "$dist"; then
    echo '서명 후 변경된 배포 파일이 게시 검증을 통과했습니다.' >&2
    exit 1
fi
mv -- "$work/install.sh" "$dist/install.sh"
(cd "$dist" && sha256sum --strict -c SHA256SUMS)
bash tests/install_smoke.sh "$dist" "${version#v}" "$work/key.der"
printf '%s\n' '서명된 배포·설치 및 검증 실패 차단 검사 통과'
