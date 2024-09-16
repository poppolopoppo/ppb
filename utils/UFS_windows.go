//go:build windows

package utils

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/poppolopoppo/ppb/internal/base"
	"golang.org/x/sys/windows"
)

/***************************************
 * Compute sha256 of a file using overlaped io
 ***************************************/

func ComputeSha256(path string, seed base.Fingerprint) ([]byte, error) {
	utf16Path, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, err
	}
	sharemode := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	handle, err := windows.CreateFile(&utf16Path[0], windows.GENERIC_READ, sharemode, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer windows.CloseHandle(handle)

	digester := base.DigesterPool.Allocate()
	defer base.DigesterPool.Release(digester)

	var overlapped windows.Overlapped
	data := base.TransientPage64KiB.Allocate()
	defer base.TransientPage64KiB.Release(data)

	var nextOverlapped windows.Overlapped
	nextData := base.TransientPage64KiB.Allocate()
	defer base.TransientPage64KiB.Release(nextData)

	// Start the first read operation.
	var done uint32
	err = windows.ReadFile(handle, (*data)[:], &done, &overlapped)
	if err != nil && err != windows.ERROR_IO_PENDING {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if _, err = digester.Write(seed[:]); err != nil {
		return nil, err
	}

	for {
		// Start the next read operation.
		err = windows.ReadFile(handle, (*nextData)[:], &done, &nextOverlapped)
		if err != nil && err != windows.ERROR_IO_PENDING {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		// Wait for the previous read operation to complete.
		var bytesReturned uint32
		err = windows.GetOverlappedResult(handle, &overlapped, &bytesReturned, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get overlapped result: %w", err)
		}

		// Write the data from the previous read operation to the hash.
		digester.Write((*data)[:bytesReturned])

		// If the next read operation returned 0 bytes, we've reached the end of the file.
		if windows.GetOverlappedResult(handle, &nextOverlapped, &bytesReturned, false) == nil {
			break
		} else if bytesReturned == 0 {
			break
		}

		// Swap the data buffers and overlapped structures for the next iteration.
		data, nextData = nextData, data
		overlapped, nextOverlapped = nextOverlapped, overlapped
	}

	return digester.Sum(nil), nil
}

/***************************************
 * Cleaning path to get the correct case is terribly expansive on Windows
 ***************************************/

var cleanPathCache = base.NewShardedMap[StringVar, string](128)

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

// /// go 1 has an internal workaround for FindFirstFileW (see syscall.FindFirstFile)
// NOTE(rsc): The Win32finddata struct is wrong for the system call:
// the two paths are each one uint16 short. Use the correct struct,
// a win32finddata1, and then copy the results out.
// There is no loss of expressivity here, because the final
// uint16, if it is used, is supposed to be a NUL, and Go doesn't need that.
// For Go 1.1, we might avoid the allocation of win32finddata1 here
// by adding a final Bug [2]uint16 field to the struct and then
// adjusting the fields in the result directly.
// --> we avoid the copy with our wrapper since this function is performance critical for us
var getFindFirstFileWSyscall = base.Memoize(func() *syscall.LazyProc {
	kernel32DLL := syscall.NewLazyDLL("kernel32.dll")
	return kernel32DLL.NewProc("FindFirstFileW")
})

// This is the actual system call structure.
// Win32finddata is what we committed to in Go 1.
type win32finddata1 struct {
	FileAttributes    uint32
	CreationTime      syscall.Filetime
	LastAccessTime    syscall.Filetime
	LastWriteTime     syscall.Filetime
	FileSizeHigh      uint32
	FileSizeLow       uint32
	Reserved0         uint32
	Reserved1         uint32
	FileName          [syscall.MAX_PATH]uint16
	AlternateFileName [14]uint16

	// The Microsoft documentation for this struct¹ describes three additional
	// fields: dwFileType, dwCreatorType, and wFinderFlags. However, those fields
	// are empirically only present in the macOS port of the Win32 API,² and thus
	// not needed for binaries built for Windows.
	//
	// ¹ https://docs.microsoft.com/en-us/windows/win32/api/minwinbase/ns-minwinbase-win32_find_dataw
	// ² https://golang.org/issue/42637#issuecomment-760715755
}

func findFirstFile1(name *uint16, data *win32finddata1) (handle syscall.Handle, err error) {
	procFindFirstFileW := getFindFirstFileWSyscall()
	r0, _, e1 := syscall.SyscallN(procFindFirstFileW.Addr(), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(data)))
	handle = syscall.Handle(r0)
	if handle == syscall.InvalidHandle {
		err = e1
	}
	return
}

// normBase returns the last element of path with correct case.
func normBase(path string) (string, error) {
	p, err := syscall.UTF16FromString(path)
	if err != nil {
		return "", err
	}

	var data win32finddata1
	h, err := findFirstFile1(&p[0], &data)
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

	cleaned := base.TransientBuffer.Allocate()
	defer base.TransientBuffer.Release(cleaned)

	cleaned.Grow(len(in))

	volName := normVolumeName(in)

	// first look for a prefix directory which was already cleaned
	dirtyOffset := len(dirty)
	for {
		var query StringVar
		separator, ok := lastIndexOfPathSeparator(dirty[:dirtyOffset])

		if ok && separator > len(volName) {
			dirtyOffset = separator
			query.Assign(dirty[:separator])
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
			var query = StringVar(dirty[:dirtyOffset])

			if realpath, ok := cleanPathCache.Get(query); ok { // some other thread might have allocated the string already
				cleaned.Reset()
				cleaned.WriteString(realpath)

			} else if realname, er := normBase(query.Get()); er == nil {
				cleaned.WriteRune(OSPathSeparator)
				cleaned.WriteString(realname)

				// store in cache for future queries, avoid querying all files all paths all the time
				cleanPathCache.Add(
					// need string copies for caching here
					StringVar(strings.ToLower(query.Get())),
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
	base.AssertErr(func() error {
		if filepath.IsAbs(in) {
			return nil
		}
		return fmt.Errorf("ufs: need absolute path -> %q", in)
	})

	// Those checks are cheap compared to the followings
	in = filepath.Clean(in)

	// Maximize cache usage by always convert to lower-case on Windows
	var query = StringVar(strings.ToLower(in))

	// /!\ EvalSymlinks() is **SUPER** expansive !
	// Try to mitigate with an ad-hoc concurrent cache
	if cleaned, ok := cleanPathCache.Get(query); ok {
		return cleaned // cache-hit: already processed
	}

	// result, err := filepath.EvalSymlinks(in)
	result, err := cacheCleanPath(in, query.Get())
	if err != nil {
		// result = in
		err = nil // if path does not exist yet
	}

	return result
}
