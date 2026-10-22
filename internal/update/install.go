package update

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	dir, err := os.MkdirTemp(filepath.Dir(s.path), "update-helper-*")
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, filepath.Base(executable))
	if err = copyFile(executable, helper, 0o700); err != nil {
		return err
	}
	plan := filepath.Join(dir, "plan.json")
	if err = storage.WriteJSON(plan, Plan{ParentPID: os.Getpid(), Target: target, Archive: s.archive, Envelope: s.envelope}); err != nil {
		return err
	}
	cmd := exec.Command(helper, "--apply-update", plan)
	hideHelper(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
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
	manifest, err := Verify(plan.Envelope, publicKey, runtime.GOOS+"-"+runtime.GOARCH)
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
	if _, err = os.Stat(newTarget); err != nil {
		return fmt.Errorf("更新包缺少应用文件: %w", err)
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
	_ = os.Remove(plan.Archive)
	if relaunch {
		cmd := exec.Command(plan.Target)
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("/usr/bin/open", "-n", plan.Target)
		}
		hideHelper(cmd)
		if err = cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Process.Release()
	}
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
