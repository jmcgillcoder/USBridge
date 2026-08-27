//go:build windows && !amd64 && !arm64

package exclusivenet

import "errors"

type wfpPolicy struct {
	interfaceIndex int
}

func installWFPPolicy(int, string) (*wfpPolicy, error) {
	return nil, errors.New("当前 Windows 架构不支持独占模式")
}

func (*wfpPolicy) Close() {}
