package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func keylessFixture(platform, version, tag string, data []byte) release {
	hash := sha256.Sum256(data)
	name := "GistManager-" + version + "-" + platform + ".zip"
	return release{TagName: tag, Name: "v" + version, Assets: []releaseAsset{
		{Name: name, URL: assetURL(tag, name), Size: int64(len(data)), Digest: "sha256:" + hex.EncodeToString(hash[:])},
		{Name: "SHA256SUMS.txt", URL: assetURL(tag, "SHA256SUMS.txt")},
	}}
}

func keylessClient(t *testing.T, fixture *release, archive []byte) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var data []byte
		switch {
		case strings.HasPrefix(r.URL.String(), githubAPI+"/releases/"):
			data, _ = json.Marshal(fixture)
		case r.URL.String() == fixture.Assets[0].URL:
			data = archive
		case r.URL.String() == assetURL(fixture.TagName, "SHA256SUMS.txt"):
			hash := sha256.Sum256(archive)
			data = fmt.Appendf(nil, "%x  %s\n", hash, fixture.Assets[0].Name)
		default:
			t.Errorf("untrusted request: %s", r.URL)
			return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
}

func TestKeylessUpdatesForEveryReleasePlatform(t *testing.T) {
	for _, platform := range []string{"windows-amd64", "windows-arm64", "darwin-amd64", "darwin-arm64"} {
		for _, checksum := range []bool{false, true} {
			for _, beta := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/checksum-%t/beta-%t", platform, checksum, beta), func(t *testing.T) {
					archive := archiveFixture(t, "fixture", "new application")
					version, tag := "1.0.1", "v1.0.1"
					if beta {
						version, tag = "1.0.1-beta.42.abc", "pre-release"
					}
					fixture := keylessFixture(platform, version, tag, archive)
					if checksum {
						fixture.Assets[0].Digest = ""
					}
					s := New(t.TempDir(), "1.0.0")
					s.Key, s.Platform, s.HTTP = "", platform, keylessClient(t, &fixture, archive)
					if beta {
						if err := s.SetChannel("beta"); err != nil {
							t.Fatal(err)
						}
					}
					if err := s.Check(context.Background()); err != nil || s.State().Status != "available" || s.State().Available != version {
						t.Fatal(err, s.State())
					}
					var progress State
					if err := s.Download(context.Background(), func(state State) { progress = state }); err != nil {
						t.Fatal(err)
					}
					if s.State().Status != "downloaded" || progress.Total != int64(len(archive)) || s.tag != tag {
						t.Fatal("update was not downloaded", s.State(), progress)
					}
					old := s.archive
					if err := s.SetChannel("stable"); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(old); !os.IsNotExist(err) || s.archive != "" || s.tag != "" || s.State().Downloaded != 0 {
						t.Fatal("channel switch retained a downloaded update")
					}
				})
			}
		}
	}
}

func TestRejectInvalidReleaseMetadata(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*release)
	}{
		{"draft", func(r *release) { r.Draft = true }},
		{"unstable", func(r *release) { r.Prerelease = true }},
		{"tag", func(r *release) { r.TagName = "v../../other" }},
		{"other-repository", func(r *release) { r.Assets[0].URL = strings.Replace(r.Assets[0].URL, Repository, "other/repo", 1) }},
		{"insecure-url", func(r *release) { r.Assets[0].URL = strings.Replace(r.Assets[0].URL, "https:", "http:", 1) }},
		{"wrong-tag-url", func(r *release) { r.Assets[0].URL = strings.Replace(r.Assets[0].URL, "/v1.0.1/", "/v1.0.0/", 1) }},
		{"oversize", func(r *release) { r.Assets[0].Size = maxDownload + 1 }},
		{"empty-size", func(r *release) { r.Assets[0].Size = 0 }},
		{"invalid-digest", func(r *release) { r.Assets[0].Digest = "sha256:invalid" }},
		{"missing-checksum", func(r *release) { r.Assets[0].Digest = ""; r.Assets = r.Assets[:1] }},
		{"foreign-checksum", func(r *release) { r.Assets[0].Digest = ""; r.Assets[1].URL = "https://untrusted.invalid/hash" }},
		{"duplicate-archive", func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := []byte("new application")
			s := New(t.TempDir(), "1.0.0")
			s.Key = ""
			r := keylessFixture(s.Platform, "1.0.1", "v1.0.1", data)
			test.mutate(&r)
			s.HTTP = keylessClient(t, &r, data)
			if err := s.Check(context.Background()); err == nil || s.State().Status != "error" || s.State().Available != "" {
				t.Fatal("invalid release accepted", err, s.State())
			}
			if err := s.Download(context.Background(), nil); err == nil {
				t.Fatal("invalid release can be downloaded")
			}
		})
	}
}

func TestUnsignedInstallerRevalidatesGitHubRelease(t *testing.T) {
	application := "GistManager.exe"
	if runtime.GOOS == "darwin" {
		application = "Gist管理器.app/Contents/MacOS/Gist管理器"
	}
	archive := archiveFixture(t, application, "new application")
	platform := runtime.GOOS + "-" + runtime.GOARCH
	for _, failure := range []string{"", "local-hash", "local-platform", "archive-tampered", "remote-replaced", "release-rolled", "network-error"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			s := New(dir, "1.0.0")
			s.Key = ""
			fixture := keylessFixture(platform, "1.0.1", "v1.0.1", archive)
			s.HTTP = keylessClient(t, &fixture, archive)
			if err := s.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := s.Download(context.Background(), nil); err != nil {
				t.Fatal(err)
			}
			target, executable := filepath.Join(dir, "installed.exe"), ""
			if runtime.GOOS == "darwin" {
				target = filepath.Join(dir, "Installed.app")
				executable = filepath.Join(target, "Contents", "MacOS", "Gist管理器")
				if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				executable = target
			}
			if err := os.WriteFile(executable, []byte("old application"), 0o700); err != nil {
				t.Fatal(err)
			}
			uninstaller := filepath.Join(dir, "uninstall.exe")
			if err := os.WriteFile(uninstaller, []byte("keep installer"), 0o700); err != nil {
				t.Fatal(err)
			}
			plan := Plan{Target: target, Archive: s.archive, Manifest: s.manifest, Tag: s.tag}
			switch failure {
			case "local-hash":
				plan.Manifest.SHA256 = strings.Repeat("0", 64)
			case "local-platform":
				plan.Manifest.Platform = "wrong-platform"
			case "archive-tampered":
				if err := os.WriteFile(plan.Archive, bytes.Repeat([]byte("x"), len(archive)), 0o600); err != nil {
					t.Fatal(err)
				}
			case "remote-replaced":
				fixture.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64)
			case "release-rolled":
				fixture.TagName, fixture.Name = "v1.0.2", "v1.0.2"
			case "network-error":
				s.HTTP = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, io.ErrUnexpectedEOF })}
			}
			path := filepath.Join(dir, "plan.json")
			data, _ := json.Marshal(plan)
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			err := applyPlan(path, "", false, s.HTTP)
			if (err != nil) != (failure != "") {
				t.Fatalf("unexpected install result: %v", err)
			}
			data, readErr := os.ReadFile(executable)
			expected := "old application"
			if failure == "" {
				expected = "new application"
			}
			if readErr != nil || string(data) != expected {
				t.Fatal("incorrect installed application", string(data), readErr)
			}
			if data, err = os.ReadFile(uninstaller); err != nil || string(data) != "keep installer" {
				t.Fatal("installation removed an unrelated file")
			}
		})
	}
}

func TestFailedCheckDiscardsPreviouslyAvailableUpdate(t *testing.T) {
	s := New(t.TempDir(), "1.0.0")
	s.Key = ""
	fixture := keylessFixture(s.Platform, "1.0.1", "v1.0.1", []byte("application"))
	s.HTTP = keylessClient(t, &fixture, nil)
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixture.Draft = true
	if err := s.Check(context.Background()); err == nil || s.manifest.Version != "" || s.State().Available != "" {
		t.Fatal("failed check kept a stale update", err, s.State())
	}
}

func TestRelaunchFailureRestoresPreviousApplication(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the fixture is intentionally an invalid Windows executable")
	}
	dir := t.TempDir()
	data := archiveFixture(t, "GistManager.exe", "invalid executable")
	envelope, key := signedFixture(t, data, "1.0.1")
	target, archive := filepath.Join(dir, "installed.exe"), filepath.Join(dir, "update.zip")
	if err := os.WriteFile(target, []byte("previous application"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plan.json")
	plan, _ := json.Marshal(Plan{Target: target, Archive: archive, Envelope: envelope})
	if err := os.WriteFile(path, plan, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPlan(path, key, true); err == nil {
		t.Fatal("invalid executable launched")
	}
	if contents, err := os.ReadFile(target); err != nil || string(contents) != "previous application" {
		t.Fatal("failed relaunch lost the old executable", err)
	}
}

// Downloads a real release and installs only into an isolated temporary folder.
func TestGitHubReleaseUpdateIntegration(t *testing.T) {
	if os.Getenv("GIST_UPDATE_LIVE_CHECK") != "1" {
		t.Skip("set GIST_UPDATE_LIVE_CHECK=1 to verify the published update")
	}
	dir := t.TempDir()
	s := New(dir, "0.0.0")
	s.Key = ""
	ctx := context.Background()
	if err := s.Check(ctx); err != nil || s.State().Status != "available" {
		t.Fatal("published release not available", err, s.State())
	}
	if err := s.Download(ctx, nil); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "GistManager.exe")
	executable := target
	if runtime.GOOS == "darwin" {
		target = filepath.Join(dir, "Gist管理器.app")
		executable = filepath.Join(target, "Contents", "MacOS", "Gist管理器")
		if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(executable, []byte("isolated placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plan.json")
	plan, _ := json.Marshal(Plan{Target: target, Archive: s.archive, Manifest: s.manifest, Tag: s.tag})
	if err := os.WriteFile(path, plan, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPlan(path, "", false); err != nil {
		t.Fatal("published archive failed to install", err)
	}
	output, err := exec.Command(executable, "--version").Output()
	if err != nil || strings.TrimSpace(string(output)) != s.manifest.Version {
		t.Fatal("installed version does not match the release", string(output), err)
	}
	t.Logf("Downloaded, verified and installed %s (%s) in a temporary folder", s.manifest.Version, s.Platform)
}
