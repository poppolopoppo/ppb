//go:build windows

package windows

import (
	"strconv"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var LogWindows = base.NewLogCategory("Windows")

var HalTag = base.MakeArchiveTag(base.MakeFourCC('W', 'I', 'N', 'X'))

func InitWindowsHAL() {
	SetupWindowsAttachDebugger()
	SetupWindowsIODetouring()
}

func InitWindowsCompile() {
	base.LogTrace(LogWindows, "build/hal/window.Init()")

	base.RegisterSerializable[WindowsPlatform]()
	base.RegisterSerializable[MsvcCompiler]()
	base.RegisterSerializable[MsvcProductInstall]()
	base.RegisterSerializable[ResourceCompiler]()
	base.RegisterSerializable[WindowsSDKInstall]()
	base.RegisterSerializable[WindowsSDK]()
	base.RegisterSerializable[ClangCompiler]()
	base.RegisterSerializable[LlvmProductInstall]()
	base.RegisterSerializable[MsvcSourceDependenciesAction]()

	AllPlatforms.Add("Win32", getWindowsPlatform_X86())
	AllPlatforms.Add("Win64", getWindowsPlatform_X64())

	compilerTypes := []CompilerType{
		COMPILER_CLANGCL,
		COMPILER_MSVC,
	}
	AllCompilerNames.Append(
		CompilerName{PersistentVar: &compilerTypes[0]},
		CompilerName{PersistentVar: &compilerTypes[1]})
}

/***************************************
 * Windows Flags
 ***************************************/

type WindowsFlags struct {
	Compiler         CompilerType
	Analyze          BoolVar
	Insider          BoolVar
	JustMyCode       BoolVar
	LlvmToolchain    BoolVar
	MscVer           MsvcVersion
	PerfSDK          BoolVar
	Permissive       BoolVar
	StackSize        base.SizeInBytes
	TranslateInclude BoolVar
	UseAfterReturn   BoolVar
	WindowsSDK       Directory
}

var GetWindowsFlags = NewCompilationFlags("WindowsCompilation", "windows-specific compilation flags", WindowsFlags{
	Analyze:          base.INHERITABLE_FALSE,
	Compiler:         COMPILER_MSVC,
	Insider:          base.INHERITABLE_FALSE,
	JustMyCode:       base.INHERITABLE_FALSE,
	LlvmToolchain:    base.INHERITABLE_TRUE,
	MscVer:           MSC_VER_LATEST,
	PerfSDK:          base.INHERITABLE_FALSE,
	Permissive:       base.INHERITABLE_FALSE,
	StackSize:        2000000,
	TranslateInclude: base.INHERITABLE_TRUE,
	UseAfterReturn:   base.INHERITABLE_FALSE,
})

func (flags *WindowsFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("Analyze", "enable/disable MSCV analysis", &flags.Analyze)
	cfv.Persistent("Compiler", "select windows compiler", &flags.Compiler)
	cfv.Persistent("Insider", "enable/disable support for pre-release toolchain", &flags.Insider)
	cfv.Persistent("JustMyCode", "enable/disable MSCV just-my-code", &flags.JustMyCode)
	cfv.Persistent("LlvmToolchain", "if enabled clang-cl will use llvm-lib and lld-link", &flags.LlvmToolchain)
	cfv.Persistent("MscVer", "select MSVC toolchain version", &flags.MscVer)
	cfv.Persistent("PerfSDK", "enable/disable Visual Studio Performance SDK", &flags.PerfSDK)
	cfv.Persistent("Permissive", "enable/disable MSCV permissive", &flags.Permissive)
	cfv.Persistent("StackSize", "set default thread stack size in bytes", &flags.StackSize)
	cfv.Persistent("TranslateInclude", "convert PCH to header units for C++20 units if enabled", &flags.TranslateInclude)
	cfv.Persistent("UseAfterReturn", "enable use-after-return when address sanitizer is enabled", &flags.UseAfterReturn)
	cfv.Persistent("WindowsSDK", "override Windows SDK install path (use latest otherwise)", &flags.WindowsSDK)
}

/***************************************
 * Windows Platform
 ***************************************/

type WindowsPlatform struct {
	PlatformRules
	CompilerType CompilerType
}

func (win *WindowsPlatform) Build(bc BuildContext) error {
	if err := win.PlatformRules.Build(bc); err != nil {
		return err
	}

	windowsFlags, err := GetWindowsFlags(bc)
	if err != nil {
		return err
	}

	win.CompilerType = windowsFlags.Compiler
	return nil
}
func (win *WindowsPlatform) Serialize(ar base.Archive) {
	ar.Serializable(&win.PlatformRules)
	ar.Serializable(&win.CompilerType)
}
func (win *WindowsPlatform) GetCompiler() BuildFactoryTyped[Compiler] {
	switch win.CompilerType {
	case COMPILER_MSVC:
		return WrapBuildFactory(func(bi BuildInitializer) (Compiler, error) {
			msvc, err := GetMsvcCompiler(win.Arch).Create(bi)
			return msvc.(Compiler), err
		})
	case COMPILER_CLANGCL:
		return WrapBuildFactory(func(bi BuildInitializer) (Compiler, error) {
			clang_cl, err := GetClangCompiler(win.Arch).Create(bi)
			return clang_cl.(Compiler), err
		})
	default:
		base.UnexpectedValue(win.CompilerType)
		return nil
	}
}

func makeWindowsPlatform(p *PlatformRules) {
	p.Os = "Windows"
	p.Defines.Append(
		"PLATFORM_PC",
		"PLATFORM_WINDOWS",
		"WIN32", "__WINDOWS__",
	)
	p.ForceIncludes.Append(UFS.Source.File("winnt_version.h"))
}
func getWindowsPlatform_X86() Platform {
	p := &WindowsPlatform{}
	p.Arch = Platform_X86.Arch
	p.Facet = NewFacet()
	p.Facet.Append(Platform_X86)
	makeWindowsPlatform(&p.PlatformRules)
	p.PlatformAlias.PlatformName = "Win32"
	p.Defines.Append("_WIN32", "__X86__")
	p.Exports.Add("Windows/Platform", "x86")
	return p
}
func getWindowsPlatform_X64() Platform {
	p := &WindowsPlatform{}
	p.Arch = Platform_X64.Arch
	p.Facet = NewFacet()
	p.Facet.Append(Platform_X64)
	makeWindowsPlatform(&p.PlatformRules)
	p.PlatformAlias.PlatformName = "Win64"
	p.Defines.Append("_WIN64", "__X64__")
	p.Exports.Add("Windows/Platform", "x64")
	return p
}

func getWindowsHostPlatform() string {
	switch strconv.IntSize {
	case 32:
		return "x86"
	case 64:
		return "x64"
	default:
		base.UnexpectedValue(strconv.IntSize)
		return ""
	}
}
