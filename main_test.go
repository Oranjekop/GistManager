package main

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Oranjekop/GistManager/internal/storage"
	"github.com/Oranjekop/GistManager/internal/update"
)

func TestPackagedNativeUpdateHelper(t *testing.T) {
	binary, feed, archive := os.Getenv("GIST_TEST_BINARY"), os.Getenv("GIST_TEST_FEED"), os.Getenv("GIST_TEST_ARCHIVE")
	if binary == "" || feed == "" || archive == "" {
		t.Skip("opt-in packaged native update integration")
	}
	dir := t.TempDir()
	data, err := os.ReadFile(feed)
	if err != nil {
		t.Fatal(err)
	}
	var envelope update.Envelope
	if err = json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	archiveData, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	archiveCopy := filepath.Join(dir, "update.zip")
	if err = os.WriteFile(archiveCopy, archiveData, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "GistManager.exe")
	if err = os.WriteFile(target, []byte("old native application"), 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "uninstall.exe")
	if err = os.WriteFile(sentinel, []byte("keep the installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(dir, "plan.json")
	if err = storage.WriteJSON(plan, update.Plan{Target: target, Archive: archiveCopy, Envelope: envelope}); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "--apply-update", plan, "--no-relaunch")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native update helper failed: %v %s", err, output)
	}
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(installed) != sha256.Sum256(original) {
		t.Fatal("installed binary differs from signed release")
	}
	if data, err = os.ReadFile(sentinel); err != nil || string(data) != "keep the installer" {
		t.Fatal("native updater changed unrelated installation files")
	}
}

func TestDefaultDataDirectoryPreservesExistingInstallation(t *testing.T) {
	for _, dev := range []bool{false, true} {
		suffix := ""
		if dev {
			suffix = "-dev"
		}
		root := t.TempDir()
		current := filepath.Join(root, "GistManager"+suffix)
		legacy := filepath.Join(root, "Gist管理器"+suffix)
		check := func(want string) {
			t.Helper()
			got, err := defaultDataDirectory(root, "GistManager", dev)
			if err != nil || got != want {
				t.Fatalf("directory = %q, %v; want %q", got, err, want)
			}
		}
		check(current)
		if err := os.Mkdir(legacy, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(legacy, "layout.json"), []byte(`{"language":"en"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		check(legacy)
		if err := os.Mkdir(current, 0o700); err != nil {
			t.Fatal(err)
		}
		check(current)
	}
}
