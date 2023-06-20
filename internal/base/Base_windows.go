//go:build windows

package base

import (
	"fmt"
	"net"
	"os"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"github.com/Showmax/go-fqdn"
)

/***************************************
 * Use a syscall to bypass Go limitation of not exposing the current thread id (for chrome trace)
 ***************************************/

var getCurrentThreadIdSyscall = Memoize(func() *syscall.LazyProc {
	kernel32DLL := syscall.NewLazyDLL("kernel32.dll")
	return kernel32DLL.NewProc("GetCurrentThreadId")
})

func isErrorErrnoNoError(err error) bool {
	errno, ok := err.(syscall.Errno)
	return ok && errno == 0
}

func GetCurrentThreadId() uintptr {
	procGetCurrentThreadId := getCurrentThreadIdSyscall()
	if ret, _, err := procGetCurrentThreadId.Call(); err == nil || isErrorErrnoNoError(err) {
		return ret
	} else {
		panic(err)
	}
}

/***************************************
 * Avoid UFS.MTime() when we already opened an *os.File, this is much faster:
 * https://github.com/loov/hrtime/blob/master/now_windows.go
 ***************************************/

func SetMTime(file *os.File, mtime time.Time) (err error) {
	mtime = mtime.Local()
	wtime := syscall.NsecToFiletime(mtime.UnixNano())
	err = syscall.SetFileTime(syscall.Handle(file.Fd()), nil, nil, &wtime)
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

/***************************************
 * Use perf counter, which give more precision than time.Now() on Windows
 * https://github.com/loov/hrtime/blob/master/now_windows.go
 ***************************************/

// precision timing
var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	procFreq    = modkernel32.NewProc("QueryPerformanceFrequency")
	procCounter = modkernel32.NewProc("QueryPerformanceCounter")

	qpcFrequency = getFrequency()
	qpcBase      = getCount()
)

// getFrequency returns frequency in ticks per second.
func getFrequency() int64 {
	var freq int64
	r1, _, _ := syscall.SyscallN(procFreq.Addr(), uintptr(unsafe.Pointer(&freq)))
	if r1 == 0 {
		Panicf("syscall failed")
	}
	return freq
}

// getCount returns counter ticks.
func getCount() int64 {
	var qpc int64
	syscall.SyscallN(procCounter.Addr(), uintptr(unsafe.Pointer(&qpc)))
	return qpc
}

// Now returns current time.Duration with best possible precision.
//
// Now returns time offset from a specific time.
// The values aren't comparable between computer restarts or between computers.
func Elapsed() time.Duration {
	return time.Duration(getCount()-qpcBase) * time.Second / (time.Duration(qpcFrequency) * time.Nanosecond)
}

// NowPrecision returns maximum possible precision for Now in nanoseconds.
func NowPrecision() float64 {
	return float64(time.Second) / (float64(qpcFrequency) * float64(time.Nanosecond))
}

/***************************************
 * Get main network interface
 ***************************************/

func GetDefaultNetInterface() (net.Interface, net.Addr, error) {
	defer LogBenchmark(LogNetwork, "GetDefaultNetInterface").Close()

	hostname, err := fqdn.FqdnHostname()
	if err != nil {
		return net.Interface{}, nil, err
	}

	LogVerbose(LogNetwork, "fully qualified domain name: %q", hostname)

	interfaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, nil, err
	}

	sort.Slice(interfaces, func(i, j int) bool {
		return interfaces[i].Index < interfaces[j].Index
	})

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 || iface.Flags&net.FlagLoopback != 0 {
			LogDebug(LogNetwork, "ignore network interface %q: %v", iface.Name, iface.Flags)
			continue
		}

		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			LogDebug(LogNetwork, "invalid network interface %q: %v", iface.Name, addrErr)
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				LogDebug(LogNetwork, "invalid network address %q: %v (%T)", iface.Name, addr, addr)
				continue
			}

			if ipNet.IP.To4() != nil || ipNet.IP.To16() != nil {
				benchmark := LogBenchmark(LogNetwork, "LookupAddr[%d] %v -> %v (%v)", iface.Index, iface.Name, ipNet.IP, iface.Flags)
				ifaceHostnames, ifaceErr := net.LookupAddr(ipNet.IP.String())
				benchmark.Close()

				if ifaceErr == nil {
					LogDebug(LogNetwork, "network inteface %q hostnames for %v: %v", iface.Name, ipNet, ifaceHostnames)
				} else {
					LogDebug(LogNetwork, "invalid lookup address for %q: %v", iface.Name, addrErr)
					continue
				}

				for _, it := range ifaceHostnames {
					if it == hostname {
						LogVerbose(LogNetwork, "select %q as main network interface (hostname=%q)", iface.Name, hostname)
						return iface, addr, nil
					}
				}
			}
		}
	}

	return net.Interface{}, nil, fmt.Errorf("failed to find main network interface")
}
