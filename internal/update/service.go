package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PublicKey optionally enables signed manifests for custom release feeds.
var PublicKey string

const Repository = "Oranjekop/GistManager"
const maxDownload = 256 << 20

type Manifest struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Notes    string `json:"notes,omitempty"`
}

type Envelope struct {
	Payload   []byte `json:"payload"`
	Signature string `json:"signature"`
}

type State struct {
	Status            string
	Current           string
	Available         string
	Message           string
	Downloaded, Total int64
}

type Service struct {
	mu       sync.Mutex
	HTTP     *http.Client
	API      string
	Key      string
	Platform string
	dir      string
	state    State
	manifest Manifest
	envelope Envelope
	tag      string
	archive  string
	busy     bool
}

func New(dir, version string) *Service {
	s := &Service{HTTP: &http.Client{Timeout: 15 * time.Minute}, API: githubAPI,
		Key: PublicKey, Platform: runtime.GOOS + "-" + runtime.GOARCH, dir: dir}
	s.state = State{Status: "idle", Current: version}
	return s
}

func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Service) DownloadPage() string {
	return "https://github.com/" + Repository + "/releases/latest"
}

// Called with mu held and no download in progress.
func (s *Service) discardUpdate() {
	s.manifest, s.envelope, s.tag = Manifest{}, Envelope{}, ""
	if s.archive != "" {
		_ = os.Remove(s.archive)
	}
	s.archive = ""
	s.state.Available, s.state.Downloaded, s.state.Total = "", 0, 0
}

func (s *Service) begin(status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return errors.New("更新操作正在进行")
	}
	s.busy = true
	s.state.Status, s.state.Message = status, ""
	return nil
}

func (s *Service) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = false
	if err != nil {
		s.state.Status, s.state.Message = "error", err.Error()
	}
}

func (s *Service) Check(ctx context.Context) (err error) {
	if err = s.begin("checking"); err != nil {
		return err
	}
	defer func() { s.finish(err) }()
	s.mu.Lock()
	s.discardUpdate()
	s.mu.Unlock()
	if strings.TrimSpace(s.Key) != "" {
		if _, err = ParsePublicKey(s.Key); err != nil {
			return err
		}
	}
	current := s.State()
	release, err := s.release(ctx, s.API+"/releases/latest")
	if err != nil {
		return err
	}
	version, err := release.version()
	if err != nil {
		return err
	}
	feedURL := ""
	for _, asset := range release.Assets {
		if strings.TrimSpace(s.Key) != "" && asset.Name == "native-update-"+s.Platform+".json" {
			feedURL = asset.URL
			break
		}
	}
	if feedURL == "" {
		if !isUpdateAvailable(current, version) {
			s.mu.Lock()
			s.state.Status, s.state.Message = "latest", "当前已是最新版本"
			s.mu.Unlock()
			return nil
		}
		manifest, manifestErr := s.releaseManifest(ctx, release, version)
		if manifestErr != nil && !errors.Is(manifestErr, errNoArchive) {
			return manifestErr
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.state.Available = version
		if errors.Is(manifestErr, errNoArchive) {
			s.state.Status, s.state.Message = "manual", "发现新版本 "+version+"，请前往 GitHub 下载并安装"
			return nil
		}
		s.manifest, s.tag = manifest, release.TagName
		s.state.Status, s.state.Message = "available", "发现新版本 "+version+"，可以下载并重启安装"
		return nil
	}
	data, err := s.get(ctx, feedURL, 2<<20)
	if err != nil {
		return err
	}
	var envelope Envelope
	if err = json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	manifest, err := Verify(envelope, s.Key, s.Platform)
	if err != nil {
		return err
	}
	if manifest.Version != version {
		return errors.New("更新清单与正式发布版本不匹配")
	}
	available := isUpdateAvailable(current, manifest.Version)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !available {
		s.state.Status, s.state.Message, s.state.Available = "latest", "当前已是最新版本", ""
		return nil
	}
	s.manifest, s.envelope = manifest, envelope
	s.state.Status, s.state.Available, s.state.Message = "available", manifest.Version, "发现新版本 "+manifest.Version
	return nil
}

func isUpdateAvailable(current State, candidate string) bool {
	// Older installations with a nonstandard version can move to a formal release.
	if !versionPattern.MatchString(current.Current) {
		return true
	}
	return Compare(candidate, current.Current) > 0
}

func ParsePublicKey(key string) (ed25519.PublicKey, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(data) != ed25519.PublicKeySize {
		return nil, errors.New("未配置有效的更新签名公钥")
	}
	return ed25519.PublicKey(data), nil
}

func Verify(envelope Envelope, key, platform string) (Manifest, error) {
	var manifest Manifest
	pub, err := ParsePublicKey(key)
	if err != nil {
		return manifest, err
	}
	sig, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil || !ed25519.Verify(pub, envelope.Payload, sig) {
		return manifest, errors.New("更新签名校验失败")
	}
	if err = json.Unmarshal(envelope.Payload, &manifest); err != nil {
		return manifest, err
	}
	u, err := url.Parse(manifest.URL)
	hash, hashErr := hex.DecodeString(manifest.SHA256)
	if err != nil || u.Scheme != "https" || u.Host == "" || manifest.Platform != platform || !versionPattern.MatchString(manifest.Version) || manifest.Size <= 0 || manifest.Size > maxDownload || hashErr != nil || len(hash) != sha256.Size {
		return Manifest{}, errors.New("更新清单无效或平台不匹配")
	}
	return manifest, nil
}

func (s *Service) get(ctx context.Context, location string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GistManager-Native")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新服务返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errors.New("更新响应超过大小限制")
	}
	return data, err
}

func (s *Service) Download(ctx context.Context, progress func(State)) (err error) {
	s.mu.Lock()
	if s.busy || s.state.Status != "available" {
		s.mu.Unlock()
		return errors.New("当前没有可下载的更新")
	}
	manifest := s.manifest
	s.busy, s.state.Status = true, "downloading"
	s.state.Total, s.state.Downloaded = manifest.Size, 0
	s.mu.Unlock()
	defer func() { s.finish(err) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GistManager-Native")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败 (HTTP %d)", resp.StatusCode)
	}
	f, err := os.CreateTemp(s.dir, "native-update-*.zip")
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	hash := sha256.New()
	written := int64(0)
	buf := make([]byte, 64<<10)
	last := time.Time{}
	reader := io.LimitReader(resp.Body, manifest.Size+1)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, err = f.Write(buf[:n]); err != nil {
				return err
			}
			hash.Write(buf[:n])
			written += int64(n)
			if written > manifest.Size {
				return errors.New("更新包大小不正确")
			}
			if time.Since(last) > 100*time.Millisecond {
				s.mu.Lock()
				s.state.Downloaded = written
				state := s.state
				s.mu.Unlock()
				if progress != nil {
					progress(state)
				}
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if written != manifest.Size || hex.EncodeToString(hash.Sum(nil)) != manifest.SHA256 {
		return errors.New("更新包校验失败，请重新下载")
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	s.mu.Lock()
	s.archive, s.state.Status, s.state.Downloaded = f.Name(), "downloaded", written
	s.state.Message = "更新已下载并通过 SHA-256 校验，可以重启安装"
	s.mu.Unlock()
	return nil
}

func Compare(a, b string) int {
	core := func(v string) [3]int {
		var nums [3]int
		for i, p := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			if i < 3 {
				nums[i], _ = strconv.Atoi(p)
			}
		}
		return nums
	}
	x, y := core(a), core(b)
	for i := range x {
		if x[i] < y[i] {
			return -1
		}
		if x[i] > y[i] {
			return 1
		}
	}
	return 0
}
