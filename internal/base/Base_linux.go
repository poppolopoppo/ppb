//go:build linux

package base

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
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

func futimens_(fd int, times *[2]unix.Timespec, flags int) (err error) {
	_, _, e1 := unix.Syscall6(unix.SYS_UTIMENSAT, uintptr(fd), uintptr(0), uintptr(unsafe.Pointer(times)), uintptr(flags), 0, 0)
	if e1 != 0 {
		err = syscall.EAGAIN
	}
	return
}

func SetMTime(file *os.File, mtime time.Time) error {
	tspec, err := unix.TimeToTimespec(mtime)
	if err != nil {
		return err
	}

	times := [2]unix.Timespec{tspec, tspec}
	err = futimens_(int(file.Fd()), &times, 0)

	if err == nil {
		Assert(func() bool {
			var info os.FileInfo
			if info, err = file.Stat(); err == nil {
				if info.ModTime() != mtime {
					LogPanic(LogBase, "SetMTime: timestamp mismatch for %q\n\tfound:\t\t%v\n\texpected:\t\t%v", file.Name(), info.ModTime(), mtime)
				}
			}
			return true
		})
	}
	return err
}

var startedAt = time.Now()

func Elapsed() time.Duration {
	return time.Since(startedAt)
}

/***************************************
 * Escape command-line argument (pass-through on linux)
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
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				return net.Interface{}, nil, err
			}

			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return iface, addr, nil
					}
				}
			}
		}
	}

	return net.Interface{}, nil, fmt.Errorf("failed to determine the main network interface")
}
