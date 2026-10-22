package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Oranjekop/GistManager/internal/storage"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(q *http.Request) (*http.Response, error) { return r(q) }

func signedFixture(t *testing.T, data []byte, version string) (Envelope, string) {
	t.Helper()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	manifest := Manifest{Version: version, Platform: runtime.GOOS + "-" + runtime.GOARCH, URL: "https://fixture.invalid/app.zip", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])}
	payload, _ := json.Marshal(manifest)
	return Envelope{Payload: payload, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))}, base64.StdEncoding.EncodeToString(pub)
}

func archiveFixture(t *testing.T, name, content string) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	f, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(content))
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestChannelsCheckDownloadAndTampering(t *testing.T) {
	archive := archiveFixture(t, "GistManager.exe", "new native executable")
	envelope, key := signedFixture(t, archive, "2.1.0")
	dir := t.TempDir()
	s := New(dir, "2.0.0-beta.2")
	s.Key = key
	corrupt := false
	s.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var data []byte
		switch {
		case strings.Contains(r.URL.Path, "/releases/"):
			if s.State().Channel == "beta" && !strings.HasSuffix(r.URL.Path, "/pre-release") {
				t.Error("beta did not use rolling release")
			}
			data = []byte(`{"assets":[{"name":"native-update-` + s.Platform + `.json","browser_download_url":"https://fixture.invalid/feed.json"}]}`)
		case r.URL.Path == "/feed.json":
			data, _ = json.MarshalIndent(envelope, "", "  ")
		default:
			data = archive
			if corrupt {
				data = bytes.Repeat([]byte("x"), len(archive))
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	if err := s.SetChannel("stable"); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state := s.State(); state.Status != "available" || state.Available != "2.1.0" {
		t.Fatal(state)
	}
	corrupt = true
	if err := s.Download(context.Background(), nil); err == nil || s.State().Status != "error" {
		t.Fatal("tampered archive accepted", err)
	}
	corrupt = false
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Download(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if s.State().Status != "downloaded" {
		t.Fatal(s.State())
	}
	if err := s.SetChannel("beta"); err != nil {
		t.Fatal(err)
	}
	if s.archive != "" || s.State().Available != "" {
		t.Fatal("old channel update retained")
	}
	if New(dir, "2.0.0").State().Channel != "beta" {
		t.Fatal("channel not persisted")
	}
	if err := s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	envelope.Payload[0] ^= 1
	if _, err := Verify(envelope, key, s.Platform); err == nil {
		t.Fatal("modified manifest accepted")
	}
}

func TestInstallVerifiedArchiveAndRejectTraversal(t *testing.T) {
	name := "GistManager.exe"
	if runtime.GOOS == "darwin" {
		name = "Gist管理器.app/Contents/MacOS/Gist管理器"
	}
	if runtime.GOOS == "linux" {
		name = "GistManager"
	}
	archive := archiveFixture(t, name, "new native executable")
	envelope, key := signedFixture(t, archive, "2.1.0")
	dir := t.TempDir()
	path := filepath.Join(dir, "update.zip")
	_ = os.WriteFile(path, archive, 0o600)
	target := filepath.Join(dir, "installed.exe")
	if runtime.GOOS == "darwin" {
		target = filepath.Join(dir, "Installed.app")
		_ = os.Mkdir(target, 0o700)
	} else {
		_ = os.WriteFile(target, []byte("old"), 0o700)
	}
	plan := filepath.Join(dir, "plan.json")
	if err := storage.WriteJSON(plan, Plan{Target: target, Archive: path, Envelope: envelope}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPlan(plan, key, false); err != nil {
		t.Fatal(err)
	}
	installed := target
	if runtime.GOOS == "darwin" {
		installed = filepath.Join(target, "Contents/MacOS/Gist管理器")
	}
	data, err := os.ReadFile(installed)
	if err != nil || string(data) != "new native executable" {
		t.Fatal(string(data), err)
	}
	bad := filepath.Join(dir, "bad.zip")
	_ = os.WriteFile(bad, archiveFixture(t, "../escape", "bad"), 0o600)
	if Extract(bad, filepath.Join(dir, "extract")) == nil {
		t.Fatal("accepted traversal")
	}
}

func TestVersionOrderingAndDisabledUpdates(t *testing.T) {
	for _, pair := range [][2]string{{"2.0.1", "2.0.0"}, {"2.0.0", "2.0.0-beta.99"}, {"2.0.0-beta.10", "2.0.0-beta.9"}, {"2.0.0-beta.1.abc", "2.0.0-beta.1"}} {
		if Compare(pair[0], pair[1]) <= 0 || Compare(pair[1], pair[0]) >= 0 {
			t.Fatal(pair)
		}
	}
	s := New(t.TempDir(), "2.0.0")
	s.Key = ""
	if err := s.Check(context.Background()); err != nil || s.State().Status != "manual" {
		t.Fatal(err, s.State())
	}
	if s.SetChannel("invalid") == nil {
		t.Fatal("accepted invalid channel")
	}
}

func TestExplicitChannelSwitchesAndDowngrade(t *testing.T) {
	for _, test := range []struct {
		current, candidate, channel string
		available                   bool
	}{
		{"2.0.0", "2.0.0-beta.10", "beta", true},
		{"2.1.0-beta.3", "2.0.0", "stable", true},
		{"2.0.0-beta.10", "2.0.0-beta.9", "beta", false},
		{"2.0.0", "1.9.0", "stable", false},
		{"2.0.0", "2.0.0", "stable", false},
	} {
		t.Run(test.current+"_to_"+test.candidate, func(t *testing.T) {
			envelope, key := signedFixture(t, []byte("package"), test.candidate)
			s := New(t.TempDir(), test.current)
			s.Key = key
			if err := s.SetChannel(test.channel); err != nil {
				t.Fatal(err)
			}
			s.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				data := []byte(`{"assets":[{"name":"native-update-` + s.Platform + `.json","browser_download_url":"https://fixture.invalid/feed.json"}]}`)
				if r.URL.Path == "/feed.json" {
					data, _ = json.Marshal(envelope)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})
			if err := s.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := s.State().Status == "available"; got != test.available {
				t.Fatalf("wrong channel switch result: %+v", s.State())
			}
		})
	}
}

func TestInstallerOnlyReleases(t *testing.T) {
	for _, test := range []struct {
		current, tag, name, channel, status, available string
	}{
		{"1.0.0", "v1.0.0", "v1.0.0", "stable", "latest", ""},
		{"1.0.0", "v1.0.1", "v1.0.1", "stable", "manual", "1.0.1"},
		{"1.0.1", "v1.0.0", "v1.0.0", "stable", "latest", ""},
		{"1.0.0-beta.1", "v1.0.0", "v1.0.0", "stable", "manual", "1.0.0"},
		{"1.0.0", "pre-release", "v1.0.0-beta.2.abc", "beta", "manual", "1.0.0-beta.2.abc"},
		{"1.0.0-beta.2.abc", "pre-release", "v1.0.0-beta.2.abc", "beta", "latest", ""},
	} {
		t.Run(test.current+"_to_"+test.name, func(t *testing.T) {
			_, key := signedFixture(t, []byte("unused"), "1.0.0")
			s := New(t.TempDir(), test.current)
			s.Key = key
			if err := s.SetChannel(test.channel); err != nil {
				t.Fatal(err)
			}
			s.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				expected := "/releases/latest"
				if test.channel == "beta" {
					expected = "/releases/tags/pre-release"
				}
				if !strings.HasSuffix(r.URL.Path, expected) {
					t.Fatal("unexpected update request", r.URL)
				}
				data, _ := json.Marshal(map[string]any{"tag_name": test.tag, "name": test.name, "assets": []any{}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
			})
			if err := s.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			state := s.State()
			if state.Status != test.status || state.Available != test.available {
				t.Fatal(state)
			}
			if err := s.Download(context.Background(), nil); err == nil {
				t.Fatal("installer-only releases must use the GitHub download page")
			}
		})
	}
}
