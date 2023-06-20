//go:build linux

package utils

import (
	"filepath"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

func GetCurrentThreadId() uintptr {
	tid, _, _ := syscall.Syscall(syscall.SYS_GETTID, 0, 0, 0)
	return tid
}

func SetMTime(file *os.File, mtime time.Time) error {
	// #TODO, see UFS_windows.go
	return base.MakeUnexpectedValueError(file, mtime)
}

var startedAt = time.Now()

func Elapsed() time.Duration {
	return time.Now() - startedAt
}

func CleanPath(in string) Directory {
	base.AssertErr(func() error {
		if filepath.IsAbs(in) {
			return nil
		}
		return fmt.Errorf("ufs: need absolute path -> %q", in)
	})

	in = filepath.Clean(in)

	if cleaned, err := filepath.Abs(in); err == nil {
		in = cleaned
	} else {
		base.LogPanicErr(err)
	}

	return SplitPath(base.result)
}

/***************************************
 * Get main network interface
 ***************************************/

func GetDefaultNetInterface() (net.Interface, net.Addr, error) {
	defaultRoute, err := net.DefaultRoute()
	if err != nil {
		return net.Interface{}, nil, err
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, nil, err
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.Equal(defaultRoute.Gateway) {
				return iface, addr, nil
			}
		}
	}

	return net.Interface{}, nil, fmt.Errorf("failed to determine the main network interface")
}
