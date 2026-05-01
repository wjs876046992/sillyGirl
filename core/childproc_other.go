//go:build !linux

package core

import "syscall"

// setChildPdeathsig 非 Linux 平台不支持 Pdeathsig，留空。
func setChildPdeathsig(attr *syscall.SysProcAttr) {
	// no-op on non-Linux
}
