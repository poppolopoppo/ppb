package windows

import (
	"os/exec"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

var LogDebugger = base.NewLogCategory("Debugger")

const DEBUGGER_RUNAS_EXE = `C:\Windows\System32\runas.exe`
const DEBUGGER_X86_VSJITDEBUGGER_EXE = `C:\Windows\System32\vsjitdebugger.exe`
const DEBUGGER_X64_VSJITDEBUGGER_EXE = `c:\Windows\SysWOW64\vsjitdebugger.exe`

func SetupWindowsAttachDebugger() {
	vsJitDebugger32 := utils.MakeFilename(DEBUGGER_X86_VSJITDEBUGGER_EXE)
	vsJitDebugger64 := utils.MakeFilename(DEBUGGER_X64_VSJITDEBUGGER_EXE)

	hasVsJitDebugger32 := vsJitDebugger32.Exists()
	hasVsJitDebugger64 := vsJitDebugger64.Exists()

	// look for gsudo
	if sudoPath, err := exec.LookPath("sudo.exe"); err == nil {
		sudo := utils.MakeFilename(sudoPath)
		internal_io.OnRunCommandWithDebugger = func(executable utils.Filename, arguments base.StringSet, options internal_io.ProcessOptions) error {

			if !strings.Contains(executable.Basename, "Win32") { // Win64 is the default, Win32 is opt-in based on basename (#TODO: better 32/64 handling)
				if hasVsJitDebugger64 {
					return runProcessWithDebugger_Sudo(sudo, vsJitDebugger64, executable, arguments, options)
				} else {
					base.LogWarning(LogDebugger, "can't attach debugger on 64 bits process")
					return internal_io.RunProcess_Vanilla(executable, arguments, options)
				}

			} else {
				if hasVsJitDebugger32 {
					return runProcessWithDebugger_Sudo(sudo, vsJitDebugger32, executable, arguments, options)
				} else {
					base.LogWarning(LogDebugger, "can't attach debugger on 32 bits process")
					return internal_io.RunProcess_Vanilla(executable, arguments, options)
				}
			}
		}
		return
	}

	// fallback to runas (which probably won't work since we don't know local admin account name)
	runAs := utils.MakeFilename(DEBUGGER_RUNAS_EXE)
	if runAs.Exists() && (hasVsJitDebugger32 || hasVsJitDebugger64) {
		internal_io.OnRunCommandWithDebugger = func(executable utils.Filename, arguments base.StringSet, options internal_io.ProcessOptions) error {

			if !strings.Contains(executable.Basename, "Win32") { // Win64 is the default, Win32 is opt-in based on basename (#TODO: better 32/64 handling)
				if hasVsJitDebugger64 {
					return runProcessWithDebugger_RunAs(runAs, vsJitDebugger64, executable, arguments, options)
				} else {
					base.LogWarning(LogDebugger, "can't attach debugger on 64 bits process")
					return internal_io.RunProcess_Vanilla(executable, arguments, options)
				}

			} else {
				if hasVsJitDebugger32 {
					return runProcessWithDebugger_RunAs(runAs, vsJitDebugger32, executable, arguments, options)
				} else {
					base.LogWarning(LogDebugger, "can't attach debugger on 32 bits process")
					return internal_io.RunProcess_Vanilla(executable, arguments, options)
				}
			}
		}
		return
	}
}

func runProcessWithDebugger_Sudo(sudo utils.Filename, vsJitDebugger utils.Filename, executable utils.Filename, arguments base.StringSet, options internal_io.ProcessOptions) error {
	commandLine := base.StringSet{vsJitDebugger.String(), executable.String()}
	commandLine.Append(arguments...)
	return internal_io.RunProcess_Vanilla(sudo, commandLine, options)
}
func runProcessWithDebugger_RunAs(runAs utils.Filename, vsJitDebugger utils.Filename, executable utils.Filename, arguments base.StringSet, options internal_io.ProcessOptions) error {
	commandLine := base.StringSet{"/user:Administrator", vsJitDebugger.String(), executable.String()}
	commandLine.Append(arguments...)
	return internal_io.RunProcess_Vanilla(runAs, commandLine, options)
}
