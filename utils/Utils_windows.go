//go:build windows

package utils

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/Showmax/go-fqdn"
)

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
					LogPanic(LogUFS, "SetMTime: timestamp mismatch for %q\n\tfound:\t\t%v\n\texpected:\t\t%v", file.Name(), info.ModTime(), mtime)
				}
			}
			return true
		})
	}
	return err
}

/***************************************
 * Cleaning path to get the correct case is terribly expansive on Windows
 ***************************************/

var cleanPathCache = NewSharedStringMap[string](128)

// normVolumeName is like VolumeName, but makes drive letter upper case.
// result of EvalSymlinks must be unique, so we have
// EvalSymlinks(`c:\a`) == EvalSymlinks(`C:\a`).
func normVolumeName(path string) string {
	volume := filepath.VolumeName(path)

	if len(volume) > 2 { // isUNC
		return volume
	}

	return strings.ToUpper(volume)
}

// normBase returns the last element of path with correct case.
func normBase(path string) (string, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	var data syscall.Win32finddata

	h, err := syscall.FindFirstFile(p, &data)
	if err != nil {
		return "", err
	}
	syscall.FindClose(h)

	return syscall.UTF16ToString(data.FileName[:]), nil
}

func cacheCleanPath(in, dirty string) (string, error) {
	// see filepath.normBase(), this version is using a cache for each sub-directory

	// skip special cases
	if in == "" || in == "." || in == `\` {
		return in, nil
	}

	cleaned := TransientBuffer.Allocate()
	defer TransientBuffer.Release(cleaned)

	cleaned.Grow(len(in))

	volName := normVolumeName(in)

	// first look for a prefix directory which was already cleaned
	dirtyOffset := len(dirty)
	for {
		var query string
		separator, ok := lastIndexOfPathSeparator(dirty[:dirtyOffset])
		if ok && separator > len(volName) {
			dirtyOffset = separator
			query = dirty[:separator]
		} else {
			dirtyOffset = len(volName)
			cleaned.WriteString(volName)
			break
		}

		if realpath, ok := cleanPathCache.Get(query); ok {
			cleaned.WriteString(realpath)
			break
		}
	}

	if dirtyOffset < len(in) {
		in = in[dirtyOffset+1:]
	} else {
		in = ``
	}

	// then clean the remaining dirty part
	var err error
	for len(in) > 0 {
		var entryName string
		if i, ok := firstIndexOfPathSeparator(in); ok {
			entryName = in[:i]
			in = in[i+1:]
			dirtyOffset += i + 1
		} else {
			dirtyOffset = len(dirty)
			entryName = in
			in = ""
		}

		if err == nil {
			query := dirty[:dirtyOffset]

			if realpath, ok := cleanPathCache.Get(query); ok { // some other thread might have allocated the string already
				cleaned.Reset()
				cleaned.WriteString(realpath)

			} else if realname, er := normBase(query); er == nil {
				cleaned.WriteRune(OSPathSeparator)
				cleaned.WriteString(realname)

				// store in cache for future queries, avoid querying all files all paths all the time
				cleanPathCache.Add(
					// need string copies for caching here
					strings.ToLower(query),
					filepath.Clean(cleaned.String()))

			} else {
				err = er
			}
		}

		if err != nil {
			cleaned.WriteRune(OSPathSeparator)
			cleaned.WriteString(entryName)
		}
	}

	return cleaned.String(), err
}

func CleanPath(in string) string {
	AssertMessage(func() bool { return filepath.IsAbs(in) }, "ufs: need absolute path -> %q", in)

	// Those checks are cheap compared to the followings
	in = filepath.Clean(in)

	// Maximize cache usage by always convert to lower-case on Windows
	query := strings.ToLower(in)

	// /!\ EvalSymlinks() is **SUPER** expansive !
	// Try to mitigate with an ad-hoc concurrent cache
	if cleaned, ok := cleanPathCache.Get(query); ok {
		return cleaned // cache-hit: already processed
	}

	// result, err := filepath.EvalSymlinks(in)
	result, err := cacheCleanPath(in, query)
	if err != nil {
		// result = in
		err = nil // if path does not exist yet
	}

	return result
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
		CommandPanicF("call failed")
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
