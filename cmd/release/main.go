package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Oranjekop/GistManager/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: go run ./cmd/release keygen|build|pack")
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "build":
		err = build(os.Args[2:])
	case "pack":
		err = pack(os.Args[2:])
	default:
		err = errors.New("unknown release command")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func keygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	privatePath := flags.String("private", ".keys/update-private.key", "Private key path (never commit)")
	publicPath := flags.String("public", "resources/update-public.key", "Public key embedded into the app")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if _, err := os.Stat(*privatePath); err == nil {
		return errors.New("private key already exists; refusing to overwrite")
	}
	if data, err := os.ReadFile(*publicPath); err == nil && strings.TrimSpace(string(data)) != "" {
		return errors.New("public key already exists; refusing to change update trust")
	}
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*privatePath), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(*privatePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(base64.StdEncoding.EncodeToString(private) + "\n")
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.WriteFile(*publicPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("Created private key at %s and public key at %s.\n", *privatePath, *publicPath)
	return nil
}

func privateKey(path string) (ed25519.PrivateKey, error) {
	encoded := os.Getenv("GIST_UPDATE_PRIVATE_KEY")
	if encoded == "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		encoded = string(data)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(data) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid GIST_UPDATE_PRIVATE_KEY (expected base64 Ed25519 private key)")
	}
	pubData, err := os.ReadFile("resources/update-public.key")
	if err != nil {
		return nil, err
	}
	pub, err := update.ParsePublicKey(string(pubData))
	if err != nil {
		return nil, err
	}
	if !ed25519.PrivateKey(data).Public().(ed25519.PublicKey).Equal(pub) {
		return nil, errors.New("signing key does not match embedded public key")
	}
	return ed25519.PrivateKey(data), nil
}

func build(args []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	platform := flags.String("platform", runtime.GOOS+"/"+runtime.GOARCH, "MyGo target platform(s)")
	version := flags.String("version", "", "Override build version")
	sign := flags.String("sign", "", "macOS signing identity")
	skipDMG := flags.Bool("skip-dmg", false, "Skip macOS DMG")
	if err := flags.Parse(args); err != nil {
		return err
	}
	original, err := os.ReadFile("mygo.json")
	if err != nil {
		return err
	}
	var config map[string]any
	if err = json.Unmarshal(original, &config); err != nil {
		return err
	}
	buildVersion, _ := config["version"].(string)
	if *version != "" {
		buildVersion = *version
	}
	if !validVersion(buildVersion) {
		return errors.New("invalid version")
	}
	if *version != "" {
		config["version"] = *version
		data, _ := json.MarshalIndent(config, "", "  ")
		if err = os.WriteFile("mygo.json", append(data, '\n'), 0o644); err != nil {
			return err
		}
		defer os.WriteFile("mygo.json", original, 0o644)
	}
	command := []string{"tool", "mygo", "build", "-platform", *platform}
	if *sign != "" {
		command = append(command, "-sign", *sign)
	}
	if *skipDMG {
		command = append(command, "-skip-dmg")
	}
	cmd := exec.Command("go", command...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		return err
	}
	return buildWindowsInstallers(*platform)
}

func validVersion(v string) bool {
	return regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v)
}

func pack(args []string) error {
	flags := flag.NewFlagSet("pack", flag.ContinueOnError)
	platform := flags.String("platform", runtime.GOOS+"-"+runtime.GOARCH, "OS-architecture")
	version := flags.String("version", "", "Release version")
	tag := flags.String("tag", "", "Formal GitHub release tag (v<version>)")
	unsigned := flags.Bool("unsigned", false, "Create a portable ZIP without an automatic-update manifest")
	keyPath := flags.String("key", ".keys/update-private.key", "Private key path when not using GIST_UPDATE_PRIVATE_KEY")
	source := flags.String("source", "", "App executable or .app directory")
	out := flags.String("out", "release", "Artifacts output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !validVersion(*version) {
		return errors.New("a valid -version is required")
	}
	if *tag == "" {
		*tag = "v" + *version
	}
	if *tag != "v"+*version {
		return errors.New("tag does not match version")
	}
	if !regexp.MustCompile(`^(windows|darwin|linux)-(amd64|arm64)$`).MatchString(*platform) {
		return errors.New("invalid platform")
	}
	var key ed25519.PrivateKey
	var err error
	if !*unsigned {
		key, err = privateKey(*keyPath)
		if err != nil {
			return err
		}
	}
	if *source == "" {
		name := "Gist管理器.exe"
		if strings.HasPrefix(*platform, "darwin-") {
			name = "Gist管理器.app"
		}
		if strings.HasPrefix(*platform, "linux-") {
			name = "gist管理器"
		}
		*source = filepath.Join("dist", *platform, name)
	}
	if err = os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	filename := "GistManager-" + *version + "-" + *platform + ".zip"
	archive := filepath.Join(*out, filename)
	root := "GistManager.exe"
	if strings.HasPrefix(*platform, "darwin-") {
		root = "Gist管理器.app"
	}
	if strings.HasPrefix(*platform, "linux-") {
		root = "GistManager"
	}
	if err = archiveApp(*source, archive, root); err != nil {
		return err
	}
	feed := filepath.Join(*out, "native-update-"+*platform+".json")
	if *unsigned {
		if err = os.Remove(feed); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("Packed %s.\n", archive)
		return nil
	}
	input, err := os.Open(archive)
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, input)
	input.Close()
	if err != nil {
		return err
	}
	manifest := update.Manifest{Version: *version, Platform: *platform, Size: size, SHA256: hex.EncodeToString(hash.Sum(nil)),
		URL: "https://github.com/" + update.Repository + "/releases/download/" + *tag + "/" + filename}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	envelope := update.Envelope{Payload: payload, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(feed, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("Signed %s (%d bytes).\n", archive, size)
	return nil
}

func archiveApp(source, target, root string) (err error) {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(output)
	defer func() {
		closeErr := writer.Close()
		fileErr := output.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = fileErr
		}
	}()
	return filepath.Walk(source, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not supported in native archive: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		name := root
		if info.IsDir() {
			relative, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			name += "/" + filepath.ToSlash(relative)
		}
		header, err := zip.FileInfoHeader(entry)
		if err != nil {
			return err
		}
		header.Name, header.Method = name, zip.Deflate
		if strings.HasPrefix(root, "Gist管理器.app") && strings.Contains(name, "/Contents/MacOS/") {
			header.SetMode(entry.Mode() | 0o111)
		}
		part, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		_, err = io.Copy(part, input)
		return err
	})
}
