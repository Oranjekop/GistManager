package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChineseInstallerPreservesApplicationResources(t *testing.T) {
	stage := t.TempDir()
	config := installerConfig{Name: "Gist管理器", Slug: "gist-manager", Identifier: "com.perzikkop.gist-manager", Version: "1.0.3"}
	output := filepath.Join(stage, config.Name+" Setup "+config.Version+" amd64.exe")
	frameworkInstaller := config.Slug + "-setup-" + config.Version + "-amd64.exe"
	for _, file := range []string{config.Name + ".exe", "icon.png", "update-public.key", filepath.Base(output), frameworkInstaller, "helper.exe"} {
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
	for _, name := range []string{config.Name + ".exe", "icon.png", "update-public.key", "helper.exe"} {
		if !strings.Contains(script, "  File "+nsisQuote(filepath.Join(stage, name))) {
			t.Errorf("installer is missing %s", name)
		}
	}
	if !strings.Contains(script, "  File /r "+nsisQuote(filepath.Join(stage, "extra resources"))) {
		t.Fatal("installer dropped a resource directory")
	}
	for _, installer := range []string{output, filepath.Join(stage, frameworkInstaller)} {
		if strings.Contains(script, "  File "+nsisQuote(installer)) {
			t.Fatal("installer included another installer", installer)
		}
	}
	for _, text := range []string{`MUI_LANGUAGE "SimpChinese"`, "创建桌面快捷方式", "运行 Gist管理器", `MUI_UNPAGE_CONFIRM`, `MUI_UNPAGE_INSTFILES`} {
		if !strings.Contains(script, text) {
			t.Errorf("missing Chinese installer UI: %s", text)
		}
	}
}

func TestUniversalWindowsInstallerSelectsNativePayload(t *testing.T) {
	root := t.TempDir()
	config := installerConfig{Name: "Gist管理器", Slug: "gist-manager", Identifier: "com.perzikkop.gist-manager", Version: "1.0.5"}
	output := filepath.Join(root, "windows-universal", config.Name+" Setup "+config.Version+".exe")
	payloads := map[string]string{}
	for _, arch := range []string{"amd64", "arm64"} {
		stage := filepath.Join(root, "windows-"+arch)
		if err := os.MkdirAll(stage, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{
			config.Name + ".exe", "icon.png", "helper.exe",
			config.Slug + "-setup-" + config.Version + "-" + arch + ".exe",
			config.Name + " Setup " + config.Version + " " + arch + ".exe",
		} {
			if err := os.WriteFile(filepath.Join(stage, name), []byte(arch), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		payloads[arch] = stage
	}
	script, err := windowsUniversalInstallerScript(config, root, output, filepath.Join(root, "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(script, "  ${If} ${IsNativeARM64}\n")
	if start < 0 {
		t.Fatal("universal installer does not detect the native ARM64 architecture")
	}
	arms := strings.SplitN(script[start:], "  ${Else}\n", 2)
	if len(arms) != 2 {
		t.Fatal("universal installer does not provide an x64 fallback")
	}
	for i, arch := range []string{"arm64", "amd64"} {
		for _, name := range []string{config.Name + ".exe", "icon.png", "helper.exe"} {
			if !strings.Contains(arms[i], "  File "+nsisQuote(filepath.Join(payloads[arch], name))) {
				t.Fatalf("%s branch is missing %s", arch, name)
			}
		}
		for _, name := range []string{
			config.Slug + "-setup-" + config.Version + "-" + arch + ".exe",
			config.Name + " Setup " + config.Version + " " + arch + ".exe",
		} {
			if strings.Contains(script, "  File "+nsisQuote(filepath.Join(payloads[arch], name))) {
				t.Fatal("universal installer included an architecture-specific installer")
			}
		}
	}
	if strings.Count(script, `WriteUninstaller "$INSTDIR\Uninstall.exe"`) != 1 ||
		!strings.Contains(script, `MUI_LANGUAGE "SimpChinese"`) {
		t.Fatal("universal installer lost the shared Chinese install/uninstall flow")
	}
	if err := os.Remove(filepath.Join(payloads["arm64"], config.Name+".exe")); err != nil {
		t.Fatal(err)
	}
	if _, err := windowsUniversalInstallerScript(config, root, output, "icon.ico"); err == nil {
		t.Fatal("universal installer accepted a missing ARM64 executable")
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
