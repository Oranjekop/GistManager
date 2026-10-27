package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/image/draw"
)

//go:embed windows-installer.nsi
var windowsInstallerTemplate string

type installerConfig struct {
	Name, Slug, Identifier, Version, Out, Icon string
}

func buildWindowsInstallers(platforms string) error {
	var targets []string
	for _, target := range strings.Split(platforms, ",") {
		target = strings.TrimSpace(target)
		if target == "windows/universal" {
			targets = []string{"windows-universal"}
			break
		}
		if strings.HasPrefix(target, "windows/") {
			targets = append(targets, strings.ReplaceAll(target, "/", "-"))
		}
	}
	if len(targets) == 0 {
		return nil
	}
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		return err
	}
	var config installerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	if config.Out == "" {
		config.Out = "dist"
	}
	if config.Icon == "" {
		config.Icon = "resources/icon.png"
	}
	tool, err := findNSIS()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "gist-chinese-installer-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	icon, err := installerIcon(config.Icon)
	if err != nil {
		return err
	}
	iconPath := filepath.Join(work, "icon.ico")
	if err := os.WriteFile(iconPath, icon, 0o600); err != nil {
		return err
	}
	for _, target := range targets {
		arch := strings.TrimPrefix(target, "windows-")
		stage, err := filepath.Abs(filepath.Join(config.Out, target))
		if err != nil {
			return err
		}
		name := config.Name + " Setup " + config.Version + " " + arch + ".exe"
		if arch == "universal" {
			name = config.Name + " Setup " + config.Version + ".exe"
		}
		output := filepath.Join(stage, name)
		var script string
		if arch == "universal" {
			if err := os.MkdirAll(stage, 0o755); err != nil {
				return err
			}
			root, absErr := filepath.Abs(config.Out)
			if absErr != nil {
				return absErr
			}
			script, err = windowsUniversalInstallerScript(config, root, output, iconPath)
		} else {
			script, err = windowsInstallerScript(config, stage, output, iconPath)
		}
		if err != nil {
			return err
		}
		scriptPath := filepath.Join(work, target+".nsi")
		// NSIS needs the UTF-8 BOM to read Chinese paths on every host locale.
		if err := os.WriteFile(scriptPath, append([]byte{0xef, 0xbb, 0xbf}, []byte(script)...), 0o600); err != nil {
			return err
		}
		command := exec.Command(tool, "-V2", scriptPath)
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("Chinese installer for %s: %w", target, err)
		}
		fmt.Printf("Built Chinese installer: %s\n", output)
	}
	return nil
}

func windowsInstallerScript(config installerConfig, stage, output, icon string) (string, error) {
	files, err := windowsPayloadFiles(config, stage, output)
	if err != nil {
		return "", err
	}
	return renderWindowsInstaller(config, files, output, icon), nil
}

func windowsUniversalInstallerScript(config installerConfig, root, output, icon string) (string, error) {
	amd64, err := windowsPayloadFiles(config, filepath.Join(root, "windows-amd64"), output)
	if err != nil {
		return "", fmt.Errorf("x64 payload: %w", err)
	}
	arm64, err := windowsPayloadFiles(config, filepath.Join(root, "windows-arm64"), output)
	if err != nil {
		return "", fmt.Errorf("ARM64 payload: %w", err)
	}
	files := "  ${If} ${IsNativeARM64}\n" + arm64 +
		"\n  ${Else}\n" + amd64 + "\n  ${EndIf}"
	return renderWindowsInstaller(config, files, output, icon), nil
}

func windowsPayloadFiles(config installerConfig, stage, output string) (string, error) {
	executable := config.Name + ".exe"
	if _, err := os.Stat(filepath.Join(stage, executable)); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return "", err
	}
	var files strings.Builder
	for _, entry := range entries {
		if entry.Name() == filepath.Base(output) ||
			entry.Name() == config.Name+" Setup "+config.Version+" amd64.exe" ||
			entry.Name() == config.Name+" Setup "+config.Version+" arm64.exe" {
			continue
		}
		// MyGo 0.7 names its generated installers after the configured slug.
		if config.Slug != "" && (entry.Name() == config.Slug+"-setup-"+config.Version+"-amd64.exe" ||
			entry.Name() == config.Slug+"-setup-"+config.Version+"-arm64.exe") {
			continue
		}
		flags := ""
		if entry.IsDir() {
			flags = "/r "
		}
		fmt.Fprintf(&files, "  File %s%s\n", flags, nsisQuote(filepath.Join(stage, entry.Name())))
	}
	return strings.TrimSuffix(files.String(), "\n"), nil
}

func renderWindowsInstaller(config installerConfig, files, output, icon string) string {
	executable := config.Name + ".exe"
	return strings.NewReplacer(
		"@NAME@", nsisQuote(config.Name),
		"@DISPLAY_NAME@", nsisEscape(config.Name),
		"@DIRECTORY@", nsisEscape(config.Name),
		"@EXECUTABLE@", nsisEscape(executable),
		"@IDENTIFIER@", nsisEscape(config.Identifier),
		"@VERSION@", nsisQuote(config.Version),
		"@OUTPUT@", nsisQuote(output),
		"@ICON@", nsisQuote(icon),
		"@FILES@", files,
	).Replace(windowsInstallerTemplate)
}

func nsisEscape(s string) string {
	return strings.NewReplacer("$", "$$", "\"", "$\\\"", "\r", "$\\r", "\n", "$\\n").Replace(s)
}

func nsisQuote(s string) string { return `"` + nsisEscape(s) + `"` }

func findNSIS() (string, error) {
	if tool, err := exec.LookPath("makensis"); err == nil {
		return tool, nil
	}
	for _, key := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		if base := os.Getenv(key); base != "" {
			path := filepath.Join(base, "NSIS", "makensis.exe")
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}
	cache, err := os.UserCacheDir()
	if err == nil {
		// MyGo downloads NSIS here before its initial installer build.
		dirs, _ := filepath.Glob(filepath.Join(cache, "mygo", "nsis-*"))
		slices.Reverse(dirs)
		for _, dir := range dirs {
			for _, relative := range []string{filepath.Join("Bin", "makensis.exe"), "makensis.exe"} {
				path := filepath.Join(dir, relative)
				if _, err := os.Stat(path); err == nil {
					return path, nil
				}
			}
		}
	}
	return "", fmt.Errorf("cannot build Chinese installer: NSIS (makensis) was not found")
}

func installerIcon(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	source, err := png.Decode(file)
	file.Close()
	if err != nil {
		return nil, err
	}
	sizes := []int{16, 32, 48, 256}
	images := make([][]byte, len(sizes))
	for i, size := range sizes {
		scaled := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), source, source.Bounds(), draw.Src, nil)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, scaled); err != nil {
			return nil, err
		}
		images[i] = encoded.Bytes()
	}
	var icon bytes.Buffer
	binary.Write(&icon, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, size := range sizes {
		dim := byte(size)
		icon.Write([]byte{dim, dim, 0, 0})
		binary.Write(&icon, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&icon, binary.LittleEndian, [2]uint32{uint32(len(images[i])), uint32(offset)})
		offset += len(images[i])
	}
	for _, data := range images {
		icon.Write(data)
	}
	return icon.Bytes(), nil
}
