//go:build linux

package hal

import (
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/hal/generic"
	"github.com/poppolopoppo/ppb/internal/hal/linux"
	"github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

func InitHAL() {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		base.LogFatal("Uname: %s", err)
	}
	base.SetCurrentHost(&base.HostPlatform{
		Id:   base.HOST_LINUX,
		Name: arrayToString(uname.Version),
	})

	base.SetEnableInteractiveShell(isInteractiveShell())

	generic.InitGenericHAL()
	linux.InitLinuxHAL()
}

func InitCompile() {
	io.FBUILD_BIN = utils.UFS.Internal.Folder("hal", "linux", "bin").File("fbuild")

	generic.InitGenericCompile()
	linux.InitLinuxCompile()
}

func isTty() bool {
	// https://stackoverflow.com/questions/68889637/is-it-possible-to-detect-if-a-writer-is-tty-or-not
	_, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	return err != nil
}

func isInteractiveShell() bool {
	// if !isTty() {
	// 	return false
	// }
	term := os.Getenv("TERM")
	switch term {
	case "xterm", "alacritty":
		return true
	default:
		return strings.HasPrefix(term, "xterm-")
	}
}

func arrayToString(x [65]int8) string {
	var buf [65]byte
	for i, b := range x {
		buf[i] = byte(b)
	}
	str := string(buf[:])
	if i := strings.Index(str, "\x00"); i != -1 {
		str = str[:i]
	}
	return str
}
