package epa

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testReleaseManifest(t *testing.T, key ed25519.PublicKey, tag, checksums string) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(fmt.Sprintf("# KRFOSS/epa release %s\n# source %s\n# key %x\n%s", tag, strings.Repeat("0", 40), sha256.Sum256(der), checksums))
}

func TestReleasePublicKey(t *testing.T) {
	key, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(der)
	if got, err := parseReleasePublicKey(encoded); err != nil || !bytes.Equal(got, key) {
		t.Fatalf("public key mismatch: %v", err)
	}
	for _, invalid := range []string{"", "broken", base64.StdEncoding.EncodeToString(key), base64.StdEncoding.EncodeToString(append(der, 0))} {
		if _, err := parseReleasePublicKey(invalid); err == nil {
			t.Fatal("accepted malformed release key")
		}
	}
}

func TestSignedReleaseManifestOpenSSL(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL is unavailable")
	}
	key, signer, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(signer)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	priv := filepath.Join(dir, "test.der")
	manifestPath := filepath.Join(dir, "SHA256SUMS")
	sigPath := filepath.Join(dir, "SHA256SUMS.sig")
	const asset, tag = "epa-linux-amd64", "v1.2.3"
	want := sha256.Sum256([]byte("fixture"))
	manifest := testReleaseManifest(t, key, tag, fmt.Sprintf("%x  %s\n", want, asset))
	if err := os.WriteFile(priv, privateDER, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(openssl, "pkeyutl", "-sign", "-keyform", "DER", "-inkey", priv, "-rawin", "-in", manifestPath, "-out", sigPath).CombinedOutput()
	if err != nil {
		t.Fatalf("OpenSSL fixture signing: %s (%v)", out, err)
	}
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := signedChecksumFor(manifest, sig, key, tag, asset)
	if err != nil || !bytes.Equal(got, want[:]) {
		t.Fatalf("Go did not verify the OpenSSL signature: %v", err)
	}
	wrongMetadata := bytes.Replace(manifest, []byte("# key "), []byte("# other-key "), 1)
	if _, err := signedChecksumFor(wrongMetadata, ed25519.Sign(signer, wrongMetadata), key, tag, asset); err == nil {
		t.Fatal("accepted missing signed key identifier")
	}
}
