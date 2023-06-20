package utils

import (
	"github.com/poppolopoppo/ppb/internal/base"
	"os"
	"os/signal"
)

var LogUtils = base.NewLogCategory("Utils")

func InitUtils() {
	base.LogTrace(LogUtils, "utils.Init()")

	setupCloseHandler()

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

/***************************************
 * Close Handler
 ***************************************/

// setupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func setupCloseHandler() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c

		base.LogWarning(LogUtils, "\r- Ctrl+C pressed in Terminal")
		CommandEnv.onExit.FireAndForget(CommandEnv)
		base.GetLogger().Purge()
		PurgeProfiling()

		os.Exit(0)
	}()
}
