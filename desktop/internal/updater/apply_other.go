//go:build !windows

package updater

import "errors"

func LaunchApply(string) error { return errors.New("自动更新仅支持 Windows") }

func RunApplyIfRequested([]string) (bool, int) { return false, 0 }
