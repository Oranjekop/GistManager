//go:build !windows

package update

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func hideHelper(*exec.Cmd) {}

func waitForParent(pid int) error {
	if pid == 0 {
		return nil
	}
	if pid == os.Getpid() {
		return errors.New("更新辅助程序不能等待自身")
	}
	deadline := time.Now().Add(60 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			return errors.New("等待应用退出超时")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
