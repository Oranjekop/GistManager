package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChineseInstallerPreservesApplicationResources(t *testing.T) {
	stage := t.TempDir()
	config := installerConfig{Name: "Gist管理器", Identifier: "com.perzikkop.gist-manager", Version: "1.0.3"}
	output := filepath.Join(stage, config.Name+" Setup "+config.Version+" amd64.exe")
	for _, file := range []string{config.Name + ".exe", "icon.png", "update-public.key", filepath.Base(output)} {
		if err := os.WriteFile(filepath.Join(stage, file), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(stage, "extra resources"), 0o700); err != nil {
		t.Fatal(err)
	}
	script, err := windowsInstallerScript(config, stage, output, filepath.Join(stage, "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{config.Name + ".exe", "icon.png", "update-public.key"} {
		if !strings.Contains(script, "  File "+nsisQuote(filepath.Join(stage, name))) {
			t.Errorf("installer is missing %s", name)
		}
	}
	if !strings.Contains(script, "  File /r "+nsisQuote(filepath.Join(stage, "extra resources"))) {
		t.Fatal("installer dropped a resource directory")
	}
	if strings.Contains(script, "  File "+nsisQuote(output)) {
		t.Fatal("installer included the framework's old installer")
	}
	for _, text := range []string{`MUI_LANGUAGE "SimpChinese"`, "创建桌面快捷方式", "运行 Gist管理器", `MUI_UNPAGE_CONFIRM`, `MUI_UNPAGE_INSTFILES`} {
		if !strings.Contains(script, text) {
			t.Errorf("missing Chinese installer UI: %s", text)
		}
	}
}

func TestReleaseCommandsRejectNonformalVersions(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("mygo.json", []byte(`{"version":"1.0.3"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.0.4-beta.1", "1.0.4-rc.1", "1.0.4+build.1"} {
		if err := build([]string{"-version", version}); err == nil || err.Error() != "invalid version" {
			t.Fatalf("nonformal build version accepted: %s: %v", version, err)
		}
		if err := pack([]string{"-unsigned", "-version", version}); err == nil {
			t.Fatalf("nonformal portable archive accepted: %s", version)
		}
	}
	if err := pack([]string{"-unsigned", "-version", "1.0.4", "-tag", "pre-release"}); err == nil || err.Error() != "tag does not match version" {
		t.Fatalf("rolling release tag accepted: %v", err)
	}
	if err := os.WriteFile("mygo.json", []byte(`{"version":"1.0.4-beta.1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := build(nil); err == nil || err.Error() != "invalid version" {
		t.Fatalf("nonformal configured build version accepted: %v", err)
	}
}
