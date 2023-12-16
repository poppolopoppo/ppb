package utils

import (
	"github.com/poppolopoppo/ppb/internal/base"
)

var LogUtils = base.NewLogCategory("Utils")

func InitUtils() {
	base.LogTrace(LogUtils, "utils.Init()")

	// write process info inside every archive serialized for compatibility handling
	base.ArchiveTags = append(base.ArchiveTags, base.StringToFourCC(PROCESS_INFO.Version))

	// register type for serialization
	base.RegisterSerializable(&buildNode{})
	base.RegisterSerializable(&Directory{})
	base.RegisterSerializable(&Filename{})

	base.RegisterSerializable(&SourceControlModifiedFiles{})
	base.RegisterSerializable(&SourceControlStatus{})
}

/***************************************
 * Expose publicly internal types
 ***************************************/

type Archive = base.Archive

var CurrentHost = base.CurrentHost
var HostIds = base.HostIds
var IfDarwin = base.IfDarwin
var IfLinux = base.IfLinux
var IfWindows = base.IfWindows
var SanitizeIdentifier = base.SanitizeIdentifier

func Inherit[T base.InheritableBase](result *T, values ...T) {
	base.Inherit(result, values...)
}
func Overwrite[T base.InheritableBase](result *T, values ...T) {
	base.Overwrite(result, values...)
}

func RegisterSerializable[T base.Serializable](value T) {
	base.RegisterSerializable(value)
}
