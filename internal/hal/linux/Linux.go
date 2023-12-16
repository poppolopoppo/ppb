//go:build linux

package linux

import (
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"

	. "github.com/poppolopoppo/ppb/utils"
)

var LogLinux = base.NewLogCategory("Linux")

var HalTag = base.MakeArchiveTag(base.MakeFourCC('L', 'I', 'N', 'X'))

func InitLinuxHAL() {

}

func InitLinuxCompile() {
	base.LogTrace(LogLinux, "build/hal/linux.Init()")

	base.RegisterSerializable[LinuxPlatform]()
	base.RegisterSerializable[LlvmProductInstall]()
	base.RegisterSerializable[LlvmCompiler]()

	compile.AllPlatforms.Add("Linux32", getLinuxPlatform_X86())
	compile.AllPlatforms.Add("Linux64", getLinuxPlatform_X64())

	compilerTypes := []CompilerType{
		COMPILER_CLANG,
		COMPILER_GCC,
	}

	compile.AllCompilerNames.Append(
		compile.CompilerName{PersistentVar: &compilerTypes[0]},
		compile.CompilerName{PersistentVar: &compilerTypes[1]})
}

/***************************************
 * Linux Flags
 ***************************************/

type LinuxFlags struct {
	Compiler          CompilerType
	LlvmVer           LlvmVersion
	DumpRecordLayouts DumpRecordLayoutsType
	StackSize         IntVar
}

var GetLinuxFlags = compile.NewCompilationFlags("LinuxCompilation", "linux-specific compilation flags", &LinuxFlags{
	Compiler:          COMPILER_CLANG,
	LlvmVer:           llvm_any,
	DumpRecordLayouts: DUMPRECORDLAYOUTS_NONE,
	StackSize:         2000000,
})

func (flags *LinuxFlags) Flags(cfv CommandFlagsVisitor) {
	cfv.Persistent("Compiler", "select windows compiler", &flags.Compiler)
	cfv.Persistent("DumpRecordLayouts", "use to investigate structure layouts", &flags.DumpRecordLayouts)
	cfv.Persistent("LlvmVer", "select LLVM toolchain version", &flags.LlvmVer)
	cfv.Persistent("StackSize", "set default thread stack size in bytes", &flags.StackSize)
}

/***************************************
 * Linux Platform
 ***************************************/

type LinuxPlatform struct {
	compile.PlatformRules
	CompilerType CompilerType
}

func (linux *LinuxPlatform) Build(bc BuildContext) (err error) {
	linux.CompilerType = GetLinuxFlags().Compiler
	return linux.PlatformRules.Build(bc)
}
func (linux *LinuxPlatform) Serialize(ar base.Archive) {
	ar.Serializable(&linux.PlatformRules)
	ar.Serializable(&linux.CompilerType)
}
func (linux *LinuxPlatform) GetCompiler() BuildFactoryTyped[compile.Compiler] {
	switch linux.CompilerType {
	case COMPILER_CLANG:
		return WrapBuildFactory(func(bi BuildInitializer) (compile.Compiler, error) {
			llvm, err := GetLlvmCompiler(linux.Arch).Create(bi)
			return llvm.(compile.Compiler), err
		})
	case COMPILER_GCC:
		base.NotImplemented("need to implement GCC support")
		return nil
	default:
		base.UnexpectedValue(linux.CompilerType)
		return nil
	}
}

func makeLinuxPlatform(p *compile.PlatformRules) {
	p.Os = "Linux"
	p.Defines.Append(
		"PLATFORM_PC",
		"PLATFORM_GLFW",
		"PLATFORM_LINUX",
		"PLATFORM_POSIX",
		"__LINUX__",
	)
}
func getLinuxPlatform_X86() compile.Platform {
	p := &LinuxPlatform{}
	p.Arch = compile.Platform_X86.Arch
	p.Facet = compile.NewFacet()
	p.Facet.Append(compile.Platform_X86)
	makeLinuxPlatform(&p.PlatformRules)
	p.PlatformAlias.PlatformName = "Linux32"
	p.Defines.Append("_LINUX32", "_POSIX32", "__X86__")
	return p
}
func getLinuxPlatform_X64() compile.Platform {
	p := &LinuxPlatform{}
	p.Arch = compile.Platform_X64.Arch
	p.Facet = compile.NewFacet()
	p.Facet.Append(compile.Platform_X64)
	makeLinuxPlatform(&p.PlatformRules)
	p.PlatformAlias.PlatformName = "Linux64"
	p.Defines.Append("_LINUX64", "_POSIX64", "__X64__")
	return p
}
