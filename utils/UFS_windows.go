//go:build windows

package utils

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * Cleaning path to get the correct case is terribly expansive on Windows
 ***************************************/

var cleanPathCache = base.NewSharedStringMap[string](128)

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

	cleaned := base.TransientBuffer.Allocate()
	defer base.TransientBuffer.Release(cleaned)

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
	base.AssertErr(func() error {
		if filepath.IsAbs(in) {
			return nil
		}
		return fmt.Errorf("ufs: need absolute path -> %q", in)
	})

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
