package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const githubAPI = "https://api.github.com/repos/" + Repository

var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)
var errNoArchive = errors.New("此版本未提供适用于当前平台的便携更新包")

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type release struct {
	TagName    string         `json:"tag_name"`
	Name       string         `json:"name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

func (s *Service) release(ctx context.Context, endpoint string) (release, error) {
	data, err := s.get(ctx, endpoint, 2<<20)
	if err != nil {
		return release{}, err
	}
	var result release
	if err = json.Unmarshal(data, &result); err != nil {
		return release{}, err
	}
	return result, nil
}

func (r release) version(channel string) (string, error) {
	version := strings.TrimPrefix(r.TagName, "v")
	if channel == "beta" && r.TagName == "pre-release" {
		version = strings.TrimPrefix(r.Name, "v")
	} else if channel != "stable" || r.TagName != "v"+version || r.Prerelease || strings.Contains(version, "-") {
		return "", errors.New("更新版本与所选通道不匹配")
	}
	if r.Draft || !versionPattern.MatchString(version) {
		return "", errors.New("更新服务返回了无效的版本")
	}
	return version, nil
}

func assetURL(tag, name string) string {
	return "https://github.com/" + Repository + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
}

func validHash(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == sha256.Size
}

// Release metadata and checksums come from the same repository over HTTPS.
// The installer fetches this metadata again rather than trusting a local plan.
func (s *Service) releaseManifest(ctx context.Context, r release, version string) (Manifest, error) {
	switch s.Platform {
	case "windows-amd64", "windows-arm64", "darwin-amd64", "darwin-arm64":
	default:
		return Manifest{}, errNoArchive
	}
	name := fmt.Sprintf("GistManager-%s-%s.zip", version, s.Platform)
	var archive, checksums *releaseAsset
	for i := range r.Assets {
		asset := &r.Assets[i]
		switch asset.Name {
		case name:
			if archive != nil {
				return Manifest{}, errors.New("更新服务返回了重复的安装包")
			}
			archive = asset
		case "SHA256SUMS.txt":
			if checksums != nil {
				return Manifest{}, errors.New("更新服务返回了重复的校验文件")
			}
			checksums = asset
		}
	}
	if archive == nil {
		return Manifest{}, errNoArchive
	}
	if archive.URL != assetURL(r.TagName, name) || archive.Size <= 0 || archive.Size > maxDownload {
		return Manifest{}, errors.New("更新包来源或大小无效")
	}
	hash := ""
	if archive.Digest != "" {
		if !strings.HasPrefix(archive.Digest, "sha256:") || !validHash(strings.TrimPrefix(archive.Digest, "sha256:")) {
			return Manifest{}, errors.New("更新包 SHA-256 摘要无效")
		}
		hash = strings.TrimPrefix(archive.Digest, "sha256:")
	} else {
		if checksums == nil || checksums.URL != assetURL(r.TagName, "SHA256SUMS.txt") {
			return Manifest{}, errors.New("更新包缺少可信的 SHA-256 校验信息")
		}
		data, err := s.get(ctx, checksums.URL, 1<<20)
		if err != nil {
			return Manifest{}, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if len(fields) != 2 || !validHash(fields[0]) {
				return Manifest{}, errors.New("更新包校验文件格式无效")
			}
			if strings.TrimPrefix(fields[1], "*") == name {
				if hash != "" {
					return Manifest{}, errors.New("更新包校验文件包含重复条目")
				}
				hash = fields[0]
			}
		}
		if hash == "" {
			return Manifest{}, errors.New("校验文件中缺少当前平台的更新包")
		}
	}
	return Manifest{Version: version, Platform: s.Platform, URL: archive.URL, Size: archive.Size, SHA256: strings.ToLower(hash)}, nil
}
