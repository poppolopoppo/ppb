//go:build darwin

package utils

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

func SetMTime(file *os.File, mtime time.Time) error {
	// #TODO, see Utils_windows.go
	return MakeUnexpectedValueError(file, mtime)
}

var startedAt = time.Now()

func Elapsed() time.Duration {
	return time.Now() - startedAt
}

func CleanPath(in string) Directory {
	AssertMessage(func() bool { return filepath.IsAbs(in) }, "ufs: need absolute path -> %q", in)

	in = filepath.Clean(in)

	if cleaned, err := filepath.Abs(in); err == nil {
		in = cleaned
	} else {
		LogPanicErr(err)
	}

	return SplitPath(result)
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
