package windows

import (
	"os/exec"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogDebugger = NewLogCategory("Debugger")

const DEBUGGER_RUNAS_EXE = `C:\Windows\System32\runas.exe`
const DEBUGGER_X86_VSJITDEBUGGER_EXE = `C:\Windows\System32\vsjitdebugger.exe`
const DEBUGGER_X64_VSJITDEBUGGER_EXE = `c:\Windows\SysWOW64\vsjitdebugger.exe`

func SetupWindowsAttachDebugger() {
	vsJitDebugger32 := MakeFilename(DEBUGGER_X86_VSJITDEBUGGER_EXE)
	vsJitDebugger64 := MakeFilename(DEBUGGER_X64_VSJITDEBUGGER_EXE)

	hasVsJitDebugger32 := vsJitDebugger32.Exists()
	hasVsJitDebugger64 := vsJitDebugger64.Exists()

	// look for gsudo
	if sudoPath, err := exec.LookPath("sudo.exe"); err == nil {
		sudo := MakeFilename(sudoPath)
		OnRunCommandWithDebugger = func(executable Filename, arguments StringSet, options ProcessOptions) error {

			if !strings.Contains(executable.Basename, "Win32") { // Win64 is the default, Win32 is opt-in based on basename (#TODO: better 32/64 handling)
				if hasVsJitDebugger64 {
					return runProcessWithDebugger_Sudo(sudo, vsJitDebugger64, executable, arguments, options)
				} else {
					LogWarning(LogDebugger, "can't attach debugger on 64 bits process")
					return RunProcess_Vanilla(executable, arguments, options)
				}

			} else {
				if hasVsJitDebugger32 {
					return runProcessWithDebugger_Sudo(sudo, vsJitDebugger32, executable, arguments, options)
				} else {
					LogWarning(LogDebugger, "can't attach debugger on 32 bits process")
					return RunProcess_Vanilla(executable, arguments, options)
				}
			}
		}
		return
	}

	// fallback to runas (which probably won't work since we don't know local admin account name)
	runAs := MakeFilename(DEBUGGER_RUNAS_EXE)
	if runAs.Exists() && (hasVsJitDebugger32 || hasVsJitDebugger64) {
		OnRunCommandWithDebugger = func(executable Filename, arguments StringSet, options ProcessOptions) error {

			if !strings.Contains(executable.Basename, "Win32") { // Win64 is the default, Win32 is opt-in based on basename (#TODO: better 32/64 handling)
				if hasVsJitDebugger64 {
					return runProcessWithDebugger_RunAs(runAs, vsJitDebugger64, executable, arguments, options)
				} else {
					LogWarning(LogDebugger, "can't attach debugger on 64 bits process")
					return RunProcess_Vanilla(executable, arguments, options)
				}

			} else {
				if hasVsJitDebugger32 {
					return runProcessWithDebugger_RunAs(runAs, vsJitDebugger32, executable, arguments, options)
				} else {
					LogWarning(LogDebugger, "can't attach debugger on 32 bits process")
					return RunProcess_Vanilla(executable, arguments, options)
				}
			}
		}
		return
	}
}

func runProcessWithDebugger_Sudo(sudo Filename, vsJitDebugger Filename, executable Filename, arguments StringSet, options ProcessOptions) error {
	commandLine := StringSet{vsJitDebugger.String(), executable.String()}
	commandLine.Append(arguments...)
	return RunProcess_Vanilla(sudo, commandLine, options)
}
func runProcessWithDebugger_RunAs(runAs Filename, vsJitDebugger Filename, executable Filename, arguments StringSet, options ProcessOptions) error {
	commandLine := StringSet{"/user:Administrator", vsJitDebugger.String(), executable.String()}
	commandLine.Append(arguments...)
	return RunProcess_Vanilla(runAs, commandLine, options)
}
