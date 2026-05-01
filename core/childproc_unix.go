//go:build !windows
// +build !windows

package core

import "syscall"

// setChildPdeathsig 设置子进程的 Pdeathsig：
// 当父进程退出时，内核自动给子进程发 SIGTERM。
// 适用于 Linux、macOS、FreeBSD 等 Unix 系统。
func setChildPdeathsig(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGTERM
}
