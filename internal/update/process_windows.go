package update

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/Oranjekop/GistManager/internal/l10n"
)

func hideHelper(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

func waitForParent(pid int) error {
	if pid == 0 {
		return nil
	}
	if pid == os.Getpid() {
		return l10n.Error("更新辅助程序不能等待自身")
	}
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(handle)
	result, err := syscall.WaitForSingleObject(handle, 60_000)
	if err != nil {
		return err
	}
	if result != syscall.WAIT_OBJECT_0 {
		return l10n.Error("等待应用退出超时")
	}
	return nil
}
