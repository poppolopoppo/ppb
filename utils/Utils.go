package utils

import (
	"github.com/poppolopoppo/ppb/internal/base"
)

var LogUtils = base.NewLogCategory("Utils")

func InitUtils() {
	base.LogTrace(LogUtils, "utils.Init()")

	// write process info inside every archive serialized for compatibility handling
	base.ArchiveTags = append(base.ArchiveTags, base.StringToFourCC(PROCESS_VERSION))

	// register type for serialization
	base.RegisterSerializable[buildNode]()
	base.RegisterSerializable[FileDependency]()
	base.RegisterSerializable[DirectoryDependency]()
	base.RegisterSerializable[SourceControlFolderStatus]()
}

/***************************************
 * Expose publicly internal types
 ***************************************/

type Archive = base.Archive

var GetCurrentHost = base.GetCurrentHost
var GetHostIds = base.GetHostIds
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

func RegisterSerializable[T any, S interface {
	*T
	base.Serializable
}]() {
	base.RegisterSerializable[T, S]()
}
