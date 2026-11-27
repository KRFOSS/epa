#!/usr/bin/env bash
# Run from the repository root with Go installed.
set -euo pipefail
export LC_ALL=C
version=${1:?release tag required}
out=${2:?output directory required}
[[ $version =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo '정식 버전 태그(vX.Y.Z)가 필요합니다.' >&2; exit 2; }
mkdir -p -- "$out"
out=$(cd -- "$out" && pwd)
if [[ -n $(ls -A -- "$out") ]]; then
    echo '출력 디렉터리는 비어 있어야 합니다.' >&2
    exit 2
fi
export CGO_ENABLED=0 GOOS=linux
assets=()
for spec in 386 amd64 armv5 armv6 armv7 arm64 loong64 mips mipsle mips64 mips64le ppc64 ppc64le riscv64 s390x; do
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
    env GOARCH="$arch" "${tuning[@]}" go build -trimpath -ldflags "-X main.version=${version#v}" -o "$out/$file" ./cmd/epa
    assets+=("$file")
done
cp -- install.sh "$out/install.sh"
(cd -- "$out" && sha256sum -- "${assets[@]}" install.sh > SHA256SUMS)
