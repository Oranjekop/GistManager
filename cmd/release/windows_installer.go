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
	Name, Identifier, Version, Out, Icon string
}

func buildWindowsInstallers(platforms string) error {
	var targets []string
	for _, target := range strings.Split(platforms, ",") {
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
		output := filepath.Join(stage, config.Name+" Setup "+config.Version+" "+arch+".exe")
		script, err := windowsInstallerScript(config, stage, output, iconPath)
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
		if entry.Name() == filepath.Base(output) {
			continue
		}
		flags := ""
		if entry.IsDir() {
			flags = "/r "
		}
		fmt.Fprintf(&files, "  File %s%s\n", flags, nsisQuote(filepath.Join(stage, entry.Name())))
	}
	return strings.NewReplacer(
		"@NAME@", nsisQuote(config.Name),
		"@DISPLAY_NAME@", nsisEscape(config.Name),
		"@DIRECTORY@", nsisEscape(config.Name),
		"@EXECUTABLE@", nsisEscape(executable),
		"@IDENTIFIER@", nsisEscape(config.Identifier),
		"@VERSION@", nsisQuote(config.Version),
		"@OUTPUT@", nsisQuote(output),
		"@ICON@", nsisQuote(icon),
		"@FILES@", strings.TrimSuffix(files.String(), "\n"),
	).Replace(windowsInstallerTemplate), nil
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
