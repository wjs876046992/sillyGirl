//go:build windows
// +build windows

package core

import "syscall"

// setChildPdeathsig Windows 不支持 Pdeathsig，留空。
// Windows 下依赖 AddNodePlugin 重载时的 processes map 清理逻辑。
func setChildPdeathsig(attr *syscall.SysProcAttr) {
	// no-op on Windows
}
