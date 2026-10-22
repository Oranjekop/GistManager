package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Oranjekop/GistManager/internal/update"
)

func TestSigningAndNativeArchive(t *testing.T) {
	t.Chdir(t.TempDir())
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir("resources", 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("resources/update-public.key", []byte(base64.StdEncoding.EncodeToString(pub)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIST_UPDATE_PRIVATE_KEY", base64.StdEncoding.EncodeToString(private))
	if err = os.WriteFile("application.exe", []byte("native executable fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = pack([]string{"-platform", "windows-amd64", "-version", "1.0.0", "-source", "application.exe"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("release", "native-update-windows-amd64.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope update.Envelope
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	manifest, err := update.Verify(envelope, base64.StdEncoding.EncodeToString(pub), "windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.0.0" || manifest.URL != "https://github.com/Oranjekop/GistManager/releases/download/v1.0.0/GistManager-1.0.0-windows-amd64.zip" {
		t.Fatal(manifest)
	}
	archive, err := zip.OpenReader("release/GistManager-1.0.0-windows-amd64.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if len(archive.File) != 1 || archive.File[0].Name != "GistManager.exe" {
		t.Fatal("unexpected archive layout")
	}
	t.Setenv("GIST_UPDATE_PRIVATE_KEY", base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)))
	if _, err = privateKey("missing"); err == nil {
		t.Fatal("mismatched signing key accepted")
	}
}
