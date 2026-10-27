package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestUniversalInstallerInstallsNativePayload(t *testing.T) {
	if os.Getenv("GIST_TEST_INSTALLER") != "1" {
		t.Skip("set GIST_TEST_INSTALLER=1 to verify an isolated NSIS installation")
	}
	var info [64]byte
	syscall.NewLazyDLL("kernel32.dll").NewProc("GetNativeSystemInfo").Call(uintptr(unsafe.Pointer(&info[0])))
	var native string
	switch binary.LittleEndian.Uint16(info[:2]) {
	case 9:
		native = "amd64"
	case 12:
		native = "arm64"
	default:
		t.Skip("installer supports x64 and ARM64 Windows")
	}
	tool, err := findNSIS()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	id := fmt.Sprintf("gist-installer-test-%d", time.Now().UnixNano())
	config := installerConfig{Name: id, Slug: id, Identifier: "com.gistmanager." + id, Version: "0.0.0"}
	for _, arch := range []string{"amd64", "arm64"} {
		stage := filepath.Join(root, "windows-"+arch)
		if err := os.MkdirAll(stage, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{config.Name + ".exe", "resource.txt"} {
			if err := os.WriteFile(filepath.Join(stage, name), []byte(arch), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	icon, err := installerIcon("../../resources/icon.png")
	if err != nil {
		t.Fatal(err)
	}
	iconPath := filepath.Join(root, "icon.ico")
	if err := os.WriteFile(iconPath, icon, 0o600); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(root, "setup.exe")
	script, err := windowsUniversalInstallerScript(config, root, installer, iconPath)
	if err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(root, "installer.nsi")
	if err := os.WriteFile(scriptPath, append([]byte{0xef, 0xbb, 0xbf}, []byte(script)...), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	compile := exec.CommandContext(ctx, tool, "-V2", scriptPath)
	compile.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile installer: %v\n%s", err, output)
	}
	installed := filepath.Join(root, "installed application")
	// NSIS requires /D and _? to be last, without quotes, even with spaces.
	run := func(runContext context.Context, path, args string) error {
		cmd := exec.CommandContext(runContext, path)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: syscall.EscapeArg(path) + args}
		return cmd.Run()
	}
	t.Cleanup(func() {
		uninstaller := filepath.Join(installed, "Uninstall.exe")
		if _, err := os.Stat(uninstaller); err == nil {
			cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cleanupCancel()
			if err := run(cleanupContext, uninstaller, " /S _?="+installed); err != nil {
				t.Errorf("remove isolated installer fixture: %v", err)
			}
		}
	})
	if err := run(ctx, installer, " /S /D="+installed); err != nil {
		t.Fatal("install isolated fixture", err)
	}
	for _, name := range []string{config.Name + ".exe", "resource.txt"} {
		data, err := os.ReadFile(filepath.Join(installed, name))
		if err != nil || string(data) != native {
			t.Fatalf("installer did not select native %s payload: %s: %q (%v)", native, name, data, err)
		}
	}
	t.Logf("Installed native %s payload and resources through the combined Chinese installer", native)
}
