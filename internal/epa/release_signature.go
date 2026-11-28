package epa

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// 정식 릴리스 빌드에 고정한 공개키입니다. 환경 변수나 내려받은 키로 교체하지 않습니다.
var releasePublicKeyDERBase64 string

var releaseSourceHeader = regexp.MustCompile(`^# source [0-9a-f]{40}$`)

func parseReleasePublicKey(encoded string) (ed25519.PublicKey, error) {
	der, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err == nil {
		key, err := x509.ParsePKIXPublicKey(der)
		if pk, ok := key.(ed25519.PublicKey); err == nil && ok && len(pk) == ed25519.PublicKeySize {
			return pk, nil
		}
	}
	return nil, LocalizedErrorf("빌드에 신뢰할 EPA 릴리스 공개키가 없습니다.", "this build has no trusted EPA release public key")
}

// 서명을 먼저 검증한 뒤에만 배포 목록에서 버전과 체크섬을 읽습니다.
func signedChecksumFor(manifest, signature []byte, key ed25519.PublicKey, tag, asset string) ([]byte, error) {
	if len(key) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, manifest, signature) {
		return nil, LocalizedErrorf("EPA 릴리스 서명이 올바르지 않습니다. 기존 EPA는 변경하지 않았습니다.", "invalid EPA release signature; existing EPA was not changed")
	}
	header, rest, ok := strings.Cut(string(manifest), "\n")
	source, rest, hasSource := strings.Cut(rest, "\n")
	keyHeader, checksums, hasKey := strings.Cut(rest, "\n")
	der, err := x509.MarshalPKIXPublicKey(key)
	if !ok || !hasSource || !hasKey || err != nil || header != "# KRFOSS/epa release "+tag ||
		!releaseSourceHeader.MatchString(source) || keyHeader != fmt.Sprintf("# key %x", sha256.Sum256(der)) {
		return nil, LocalizedErrorf("서명된 EPA 배포 목록이 요청한 릴리스와 일치하지 않습니다: %s", "signed EPA release manifest does not match the requested release: %s", tag)
	}
	return checksumFor([]byte(checksums), asset)
}
