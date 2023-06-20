//go:build windows

package hal

import (
	"fmt"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
	"os"
	"syscall"

	"github.com/poppolopoppo/ppb/internal/hal/generic"
	"github.com/poppolopoppo/ppb/internal/hal/windows"
	"github.com/poppolopoppo/ppb/internal/io"
)

var LogHAL = base.NewLogCategory("HAL")

func osVersion() string {
	v, err := syscall.GetVersion()
	if err != nil {
		return "0.0"
	}
	major := uint8(v)
	minor := uint8(v >> 8)
	build := uint16(v >> 16)
	return fmt.Sprintf("%d.%d build %d", major, minor, build)
}

func setConsoleMode() bool {
	stdout := syscall.Handle(os.Stdout.Fd())

	var originalMode uint32
	syscall.GetConsoleMode(stdout, &originalMode)
	originalMode |= 0x0004

	getConsoleMode := syscall.MustLoadDLL("kernel32").MustFindProc("SetConsoleMode")
	ret, _, err := getConsoleMode.Call(uintptr(stdout), uintptr(originalMode))

	if ret == 1 {
		return true
	}

	base.LogVerbose(LogHAL, "failed to set console mode with %v", err)
	return false
}

func InitHAL() {
	base.SetCurrentHost(&base.HostPlatform{
		Id:   base.HOST_WINDOWS,
		Name: "Windows " + osVersion(),
	})

	base.SetEnableInteractiveShell(setConsoleMode())

	generic.InitGenericHAL()
	windows.InitWindowsHAL()
}

func InitCompile() {
	io.FBUILD_BIN = utils.UFS.Internal.Folder("hal", "windows", "bin").File("FBuild.exe")

	generic.InitGenericCompile()
	windows.InitWindowsCompile()
}
