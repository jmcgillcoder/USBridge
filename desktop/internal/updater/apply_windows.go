//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

const applyArgument = "--usbridge-apply-update"

func LaunchApply(downloadedPath string) error {
	target, err := os.Executable()
	if err != nil {
		return fmt.Errorf("读取程序路径失败：%w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	probe, err := os.CreateTemp(filepath.Dir(target), ".usbridge-update-check-*")
	if err != nil {
		return fmt.Errorf("当前程序目录不可写，请将 USBridge 移到可写目录后重试：%w", err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	_ = os.Remove(probePath)
	command := exec.Command(downloadedPath,
		applyArgument,
		"--update-target="+target,
		"--update-pid="+strconv.Itoa(os.Getpid()),
	)
	command.Dir = filepath.Dir(downloadedPath)
	command.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动更新程序失败：%w", err)
	}
	return command.Process.Release()
}

func RunApplyIfRequested(arguments []string) (bool, int) {
	if len(arguments) == 0 || arguments[0] != applyArgument {
		return false, 0
	}
	var target string
	var processID int
	for _, argument := range arguments[1:] {
		switch {
		case strings.HasPrefix(argument, "--update-target="):
			target = strings.TrimPrefix(argument, "--update-target=")
		case strings.HasPrefix(argument, "--update-pid="):
			processID, _ = strconv.Atoi(strings.TrimPrefix(argument, "--update-pid="))
		}
	}
	if target == "" || processID <= 0 {
		return true, 2
	}
	if err := applyUpdate(target, uint32(processID)); err != nil {
		fmt.Fprintln(os.Stderr, "USBridge update:", err)
		return true, 1
	}
	return true, 0
}

func applyUpdate(target string, processID uint32) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, processID)
	if err == nil {
		defer windows.CloseHandle(handle)
		result, waitErr := windows.WaitForSingleObject(handle, 30_000)
		if waitErr != nil {
			return fmt.Errorf("等待旧版本退出失败：%w", waitErr)
		}
		if result == uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("等待旧版本退出超时")
		}
	}
	source, err := os.Executable()
	if err != nil {
		return err
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	temporary := target + ".update"
	if err := os.WriteFile(temporary, contents, 0o755); err != nil {
		return fmt.Errorf("写入新版程序失败：%w", err)
	}
	temporaryPtr, _ := windows.UTF16PtrFromString(temporary)
	targetPtr, _ := windows.UTF16PtrFromString(target)
	if err := windows.MoveFileEx(temporaryPtr, targetPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("替换旧版程序失败：%w", err)
	}
	command := exec.Command(target)
	command.Dir = filepath.Dir(target)
	if err := command.Start(); err != nil {
		return fmt.Errorf("重启 USBridge 失败：%w", err)
	}
	return command.Process.Release()
}
