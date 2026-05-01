//go:build linux

package core

import "syscall"

// setChildPdeathsig 设置子进程的 Pdeathsig：
// 当父进程退出时，内核自动给子进程发 SIGTERM。
func setChildPdeathsig(attr *syscall.SysProcAttr) {
	attr.Pdeathsig = syscall.SIGTERM
}
