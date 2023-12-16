package utils

import (
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

const PROCESS_VERSION = "0.1.0"

var PROCESS_INFO = getExecutableInfo()

func GenericFileFingerprint(src Filename, seed base.Fingerprint) (base.Fingerprint, error) {
	var result base.Fingerprint
	err := UFS.OpenFile(src, func(rd *os.File) (err error) {
		result, err = base.ReaderFingerprint(rd, seed)
		return
	})
	return result, err
}

/***************************************
 * Process Fingerprint
 ***************************************/

type ProcessInfo struct {
	Path      string
	Version   string
	Timestamp time.Time
	Checksum  base.Future[base.Fingerprint]
}

func (x ProcessInfo) String() string {
	return fmt.Sprintf("%v-%v-%v", x.Path, x.Version, x.Checksum.Join().Success().ShortString())
}

var GetProcessSeed = base.Memoize(func() base.Fingerprint {
	result := PROCESS_INFO.Checksum.Join()
	base.LogPanicIfFailed(base.LogFingerprint, result.Failure())
	return result.Success()
})

func getExecutableInfo_FromFile() (result ProcessInfo) {
	if x, ok := debug.ReadBuildInfo(); ok {
		if x.Main.Path != "" {
			result.Path = x.Main.Path
			result.Version = x.Main.Version
		} else {
			result.Path = UFS.Executable.String()
			result.Version = PROCESS_VERSION
		}

		if base.DEBUG_ENABLED {
			// do not invalidate database checksum when building in DEBUG to ease bug reproduction
			result.Timestamp = time.Date(1985, 4, 5, 8, 30, 45, 100, time.UTC)
			result.Checksum = base.MakeFuture(func() (base.Fingerprint, error) {
				return base.StringFingerprint(result.Path), nil
			})
		} else {
			result.Timestamp = UFS.MTime(UFS.Executable)
			result.Checksum = base.MakeFuture(func() (base.Fingerprint, error) {
				return FileFingerprint(UFS.Executable, base.Fingerprint{})
			})
		}

	} else {
		base.LogPanic(base.LogFingerprint, "no module build info!")
	}
	// round up timestamp to millisecond, see ArchiveBinaryReader/Writer.Time()
	result.Timestamp = time.UnixMilli(result.Timestamp.UnixMilli())
	return
}

// can disable executable seed for debugging
const process_enable_executable_seed = true

func getExecutableInfo() (result ProcessInfo) {
	if process_enable_executable_seed {
		result = getExecutableInfo_FromFile()
	}
	return result
}
