//go:build linux

package base

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

/***************************************
 * Thread id
 ***************************************/

func GetCurrentThreadId() uintptr {
	tid, _, _ := syscall.Syscall(syscall.SYS_GETTID, 0, 0, 0)
	return tid
}

/***************************************
 * Time helpers
 ***************************************/

func SetMTime(file *os.File, mtime time.Time) error {
	// #TODO, see UFS_windows.go
	return MakeUnexpectedValueError(file, mtime)
}

var startedAt = time.Now()

func Elapsed() time.Duration {
	return time.Now() - startedAt
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
