package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestPackagedRestartShowsMainWindow(t *testing.T) {
	binary := os.Getenv("GIST_TEST_BINARY")
	if binary == "" {
		t.Skip("set GIST_TEST_BINARY to verify the packaged restart window")
	}
	for _, hidden := range []bool{false, true} {
		name := "normal-launch"
		if hidden {
			name = "legacy-hidden-launch"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			screenshot := filepath.Join(t.TempDir(), "native-window.png")
			command := exec.CommandContext(ctx, binary, "--demo", "--smoke", screenshot)
			// Previous update helpers start the new GUI with SW_HIDE.
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: hidden}
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			windowVisible := observeMainWindow(uint32(command.Process.Pid))
			finished := make(chan error, 1)
			go func() { finished <- command.Wait() }()
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			visible := false
			for {
				select {
				case err := <-finished:
					if err != nil {
						t.Fatal("packaged application failed", err)
					}
					if !visible {
						t.Fatal("application ran without showing its main window")
					}
					if _, err := os.Stat(screenshot); err != nil {
						t.Fatal("native demo did not finish rendering", err)
					}
					return
				case <-tick.C:
					visible = visible || windowVisible()
				}
			}
		})
	}
}

func observeMainWindow(pid uint32) func() bool {
	user32 := syscall.NewLazyDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	getProcess := user32.NewProc("GetWindowThreadProcessId")
	isVisible := user32.NewProc("IsWindowVisible")
	getTitle := user32.NewProc("GetWindowTextW")
	found := false
	callback := syscall.NewCallback(func(window, _ uintptr) uintptr {
		var owner uint32
		getProcess.Call(window, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		title := make([]uint16, 256)
		getTitle.Call(window, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		visible, _, _ := isVisible.Call(window)
		if visible != 0 && syscall.UTF16ToString(title) == "Gist管理器" {
			found = true
			return 0
		}
		return 1
	})
	return func() bool {
		enumWindows.Call(callback, 0)
		return found
	}
}
