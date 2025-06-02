package utils

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

const PROCESS_VERSION = "0.4.0"

/***************************************
 * Process Fingerprint
 ***************************************/

type ProcessInfo struct {
	Path      string
	Version   string
	Timestamp time.Time
	Checksum  base.Fingerprint
}

func (x ProcessInfo) String() string {
	return fmt.Sprintf("%v-%v-%v", x.Path, x.Version, x.Checksum.ShortString())
}

var GetProcessInfo = base.Memoize(func() *ProcessInfo {
	pi := getExecutableInfo()
	base.LogTrace(LogCommand, "process info: %q v%v [%v] %v", pi.Path, pi.Version, pi.Checksum, pi.Timestamp)
	return &pi
})

func GetProcessSeed() base.Fingerprint {
	return GetProcessInfo().Checksum
}

func getExecutableInfo_FromFile() (result ProcessInfo) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path != "" {
			result.Path = bi.Main.Path
			result.Version = bi.Main.Version
		} else {
			result.Path = UFS.Executable.String()
			result.Version = PROCESS_VERSION
		}

		if base.DEBUG_ENABLED {
			// do not invalidate database checksum when building in DEBUG to ease bug reproduction
			result.Timestamp = time.Date(1985, 4, 5, 8, 30, 45, 100, time.UTC)
			result.Checksum = base.StringFingerprint("static fingerprint for debug builds")

		} else {
			result.Timestamp = UFS.MTime(UFS.Executable)

			digester := base.DigesterPool.Allocate()
			defer base.DigesterPool.Release(digester)

			digester.Write(base.UnsafeBytesFromString(bi.Main.Path))
			digester.Write(base.UnsafeBytesFromString(bi.Main.Version))
			digester.Write(base.UnsafeBytesFromString(bi.Main.Sum))

			for _, it := range bi.Deps {
				digester.Write(base.UnsafeBytesFromString(it.Path))
				digester.Write(base.UnsafeBytesFromString(it.Version))
				digester.Write(base.UnsafeBytesFromString(it.Sum))
			}

			copy(result.Checksum[:], digester.Sum(nil))
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
