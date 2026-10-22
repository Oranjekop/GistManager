package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Oranjekop/GistManager/internal/storage"
)

type Plan struct {
	ParentPID int      `json:"parentPID,omitempty"`
	Target    string   `json:"target"`
	Archive   string   `json:"archive"`
	Envelope  Envelope `json:"envelope"`
	Manifest  Manifest `json:"manifest,omitempty"`
	Tag       string   `json:"releaseTag,omitempty"`
}

// StartInstaller runs a copy of this native executable, so Windows can replace
// the running application after its window has closed.
func (s *Service) StartInstaller() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy || s.state.Status != "downloaded" || s.archive == "" {
		return errors.New("更新尚未下载完成")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	target, err := InstallTarget(executable)
	if err != nil {
		return err
	}
	probe, err := os.CreateTemp(filepath.Dir(target), ".gist-update-check-*")
	if err != nil {
		return fmt.Errorf("应用目录不可写，请使用安装包手动更新: %w", err)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	dir, err := os.MkdirTemp(filepath.Dir(s.path), "update-helper-*")
	if err != nil {
		return err
	}
	started := false
	defer func() {
		if !started {
			_ = os.RemoveAll(dir)
		}
	}()
	helper := filepath.Join(dir, filepath.Base(executable))
	if err = copyFile(executable, helper, 0o700); err != nil {
		return err
	}
	plan := filepath.Join(dir, "plan.json")
	if err = storage.WriteJSON(plan, Plan{ParentPID: os.Getpid(), Target: target, Archive: s.archive, Envelope: s.envelope, Manifest: s.manifest, Tag: s.tag}); err != nil {
		return err
	}
	cmd := exec.Command(helper, "--apply-update", plan)
	hideHelper(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	started = true
	s.state.Status, s.state.Message = "installing", "正在退出并安装更新"
	return nil
}

func InstallTarget(executable string) (string, error) {
	executable, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
		if !strings.HasSuffix(bundle, ".app") {
			return "", errors.New("请在安装后的 .app 中更新")
		}
		return bundle, nil
	}
	return executable, nil
}

func ApplyPlan(path, publicKey string, relaunch bool) error {
	return applyPlan(path, publicKey, relaunch, &http.Client{Timeout: 90 * time.Second})
}

func verifyPlan(ctx context.Context, plan Plan, key string, client *http.Client) (Manifest, error) {
	platform := runtime.GOOS + "-" + runtime.GOARCH
	if len(plan.Envelope.Payload) != 0 || plan.Envelope.Signature != "" {
		return Verify(plan.Envelope, key, platform)
	}
	if plan.Tag != "v"+plan.Manifest.Version && plan.Tag != "pre-release" || !versionPattern.MatchString(plan.Manifest.Version) {
		return Manifest{}, errors.New("更新计划包含无效的版本")
	}
	// The API origin is fixed here: a local plan cannot supply its own authority.
	s := &Service{HTTP: client, Platform: platform}
	r, err := s.release(ctx, githubAPI+"/releases/tags/"+url.PathEscape(plan.Tag))
	if err != nil {
		return Manifest{}, fmt.Errorf("安装前无法确认 GitHub 发布信息，请重新检查更新: %w", err)
	}
	channel := "stable"
	if plan.Tag == "pre-release" {
		channel = "beta"
	}
	version, err := r.version(channel)
	if err != nil || r.TagName != plan.Tag || version != plan.Manifest.Version {
		return Manifest{}, errors.New("发布版本已变化，请重新检查更新")
	}
	manifest, err := s.releaseManifest(ctx, r, version)
	if err != nil {
		return Manifest{}, err
	}
	expected := plan.Manifest
	if manifest.Platform != expected.Platform || manifest.URL != expected.URL || manifest.Size != expected.Size || manifest.SHA256 != expected.SHA256 {
		return Manifest{}, errors.New("更新包与 GitHub 发布信息不一致，请重新下载")
	}
	return manifest, nil
}

func applyPlan(path, publicKey string, relaunch bool, client *http.Client) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var plan Plan
	if err = json.Unmarshal(data, &plan); err != nil {
		return err
	}
	if err = waitForParent(plan.ParentPID); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	manifest, err := verifyPlan(ctx, plan, publicKey, client)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(plan.Target) || !filepath.IsAbs(plan.Archive) {
		return errors.New("更新路径必须是绝对路径")
	}
	f, err := os.Open(plan.Archive)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, manifest.Size+1))
	f.Close()
	if err != nil {
		return err
	}
	if n != manifest.Size || hex.EncodeToString(hash.Sum(nil)) != manifest.SHA256 {
		return errors.New("更新包在安装前被修改")
	}
	work, err := os.MkdirTemp(filepath.Dir(plan.Target), ".gist-update-*")
	if err != nil {
		return err
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			os.RemoveAll(work)
		}
	}()
	if err = Extract(plan.Archive, work); err != nil {
		return err
	}
	newTarget := filepath.Join(work, "GistManager.exe")
	if runtime.GOOS == "darwin" {
		newTarget = filepath.Join(work, "Gist管理器.app")
	}
	if runtime.GOOS == "linux" {
		newTarget = filepath.Join(work, "GistManager")
	}
	entry := newTarget
	if runtime.GOOS == "darwin" {
		entry = filepath.Join(newTarget, "Contents", "MacOS", "Gist管理器")
	}
	info, err := os.Stat(entry)
	if err != nil {
		return fmt.Errorf("更新包缺少应用文件: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("更新包中的应用文件无效")
	}
	backup := filepath.Join(work, "previous")
	deadline := time.Now().Add(60 * time.Second)
	for {
		err = os.Rename(plan.Target, backup)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("无法替换应用: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err = os.Rename(newTarget, plan.Target); err != nil {
		if rollback := os.Rename(backup, plan.Target); rollback != nil {
			// Keep the backup available if both the swap and rollback fail.
			keepBackup = true
			return fmt.Errorf("更新和恢复失败，备份位于 %s: %w", backup, rollback)
		}
		return err
	}
	if relaunch {
		cmd := exec.Command(plan.Target)
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("/usr/bin/open", "-n", plan.Target)
		}
		hideHelper(cmd)
		if err = cmd.Start(); err != nil {
			if moveErr := os.Rename(plan.Target, newTarget); moveErr != nil {
				keepBackup = true
				return fmt.Errorf("新版本无法启动，原版本备份位于 %s: %w", backup, err)
			}
			if rollback := os.Rename(backup, plan.Target); rollback != nil {
				keepBackup = true
				return fmt.Errorf("新版本无法启动且恢复失败，备份位于 %s: %w", backup, rollback)
			}
			return fmt.Errorf("新版本无法启动，已恢复原版本: %w", err)
		}
		_ = cmd.Process.Release()
	}
	_ = os.Remove(plan.Archive)
	return nil
}

func Extract(archive, target string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer r.Close()
	var total uint64
	for _, file := range r.File {
		name := filepath.FromSlash(file.Name)
		if !filepath.IsLocal(name) || strings.Contains(file.Name, "\\") || file.Mode()&os.ModeSymlink != 0 {
			return errors.New("更新包包含不安全的路径")
		}
		total += file.UncompressedSize64
		if total > 512<<20 || len(r.File) > 20000 {
			return errors.New("更新包解压大小超过限制")
		}
		path := filepath.Join(target, name)
		if file.FileInfo().IsDir() {
			if err = os.MkdirAll(path, 0o700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		mode := file.Mode().Perm() & 0o755
		if mode == 0 {
			mode = 0o600
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			input.Close()
			return err
		}
		_, err = io.Copy(output, io.LimitReader(input, int64(file.UncompressedSize64)+1))
		input.Close()
		closeErr := output.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func copyFile(from, to string, mode os.FileMode) error {
	input, err := os.Open(from)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}
