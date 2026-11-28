#!/usr/bin/env bash
# Go가 설치된 환경에서 저장소 루트로 이동한 뒤 실행합니다.
set -euo pipefail
export LC_ALL=C
source "$(dirname -- "${BASH_SOURCE[0]}")/release_format.sh"
version=${1:?release tag required}
out=${2:?output directory required}
source_sha=${3:-$(git rev-parse HEAD)}
[[ $version =~ ^v(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})$ ]] || { echo '정식 버전 태그(vX.Y.Z)가 필요합니다.' >&2; exit 2; }
[[ $source_sha =~ ^[0-9a-f]{40}$ && $source_sha == "$(git rev-parse HEAD)" ]] || { echo '빌드 소스 커밋이 요청한 SHA와 일치하지 않습니다.' >&2; exit 2; }
key_digest=$(release_public_key | sha256sum)
key_digest=${key_digest%% *}
mkdir -p -- "$out"
out=$(cd -- "$out" && pwd)
if [[ -n $(ls -A -- "$out") ]]; then
    echo '출력 디렉터리는 비어 있어야 합니다.' >&2
    exit 2
fi
export CGO_ENABLED=0 GOOS=linux
for spec in "${release_arches[@]}"; do
    arch=$spec
    tuning=()
    case "$spec" in
        armv*) arch=arm; tuning=("GOARM=${spec#armv},softfloat") ;;
        386) tuning=(GO386=softfloat) ;;
        amd64) tuning=(GOAMD64=v1) ;;
        arm64) tuning=(GOARM64=v8.0) ;;
        mips|mipsle) tuning=(GOMIPS=softfloat) ;;
        mips64|mips64le) tuning=(GOMIPS64=softfloat) ;;
        ppc64|ppc64le) tuning=(GOPPC64=power8) ;;
        riscv64) tuning=(GORISCV64=rva20u64) ;;
    esac
    file=epa-linux-$spec
    printf '빌드: %s\n' "$file"
    env GOARCH="$arch" "${tuning[@]}" go build -trimpath -buildvcs=false \
        -ldflags "-X main.version=${version#v} -X github.com/KRFOSS/epa/internal/epa.releasePublicKeyDERBase64=$EPA_RELEASE_PUBLIC_KEY" \
        -o "$out/$file" ./cmd/epa
done
cp -- install.sh "$out/install.sh"
(cd -- "$out" && release_manifest "$version" "$source_sha" "$key_digest" > SHA256SUMS)
