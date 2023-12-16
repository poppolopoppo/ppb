//go:build darwin

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
 * Escape command-line argument (pass-through on darwin)
 ***************************************/

func EscapeCommandLineArg(a string) string {
	return a
}

/***************************************
 * Get main network interface
 ***************************************/

func GetDefaultNetInterface() (net.Interface, net.Addr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, nil, err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.DefaultMask() != nil {
				return iface, addr, nil
			}
		}
	}

	return net.Interface{}, nil, fmt.Errorf("failed to determine the main network interface")
}
