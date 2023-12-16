//go:build linux

package io

import "syscall"

func newProcessGroupSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}
