package windows

import (
	"fmt"
	"github.com/poppolopoppo/ppb/internal/base"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/compile"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

type ResourceCompiler struct {
	compile.CompilerRules
}

func (res *ResourceCompiler) GetCompiler() *compile.CompilerRules { return &res.CompilerRules }

func (res *ResourceCompiler) Extname(compile.PayloadType) string {
	return ".res"
}

func (res *ResourceCompiler) CppRtti(*compile.Facet, bool)              {}
func (res *ResourceCompiler) CppStd(*compile.Facet, compile.CppStdType) {}

func (res *ResourceCompiler) DebugSymbols(*compile.Unit) {}

func (res *ResourceCompiler) AllowCaching(*compile.Unit, compile.PayloadType) action.CacheModeType {
	return action.CACHE_NONE
}
func (res *ResourceCompiler) AllowDistribution(*compile.Unit, compile.PayloadType) action.DistModeType {
	return action.DIST_NONE
}
func (res *ResourceCompiler) AllowResponseFile(*compile.Unit, compile.PayloadType) compile.CompilerSupportType {
	return compile.COMPILERSUPPORT_UNSUPPORTED
}
func (res *ResourceCompiler) AllowEditAndContinue(*compile.Unit, compile.PayloadType) compile.CompilerSupportType {
	return compile.COMPILERSUPPORT_UNSUPPORTED
}

func (res *ResourceCompiler) Define(facet *compile.Facet, def ...string) {
	for _, x := range def {
		facet.AddCompilationFlag(fmt.Sprintf("/d%s", x))
	}
}
func (res *ResourceCompiler) Link(*compile.Facet, compile.LinkType) {}
func (res *ResourceCompiler) PrecompiledHeader(*compile.Unit) {
}
func (res *ResourceCompiler) Sanitizer(*compile.Facet, compile.SanitizerType) {}

func (res *ResourceCompiler) ForceInclude(*compile.Facet, ...utils.Filename) {}
func (res *ResourceCompiler) IncludePath(facet *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		facet.AddCompilationFlag(fmt.Sprintf("/i%v", x))
	}
}
func (res *ResourceCompiler) ExternIncludePath(facet *compile.Facet, dirs ...utils.Directory) {
	res.IncludePath(facet, dirs...)
}
func (res *ResourceCompiler) SystemIncludePath(facet *compile.Facet, dirs ...utils.Directory) {
	res.IncludePath(facet, dirs...)
}
func (res *ResourceCompiler) Library(*compile.Facet, ...utils.Filename)      {}
func (res *ResourceCompiler) LibraryPath(*compile.Facet, ...utils.Directory) {}
func (res *ResourceCompiler) CreateAction(_ *compile.Unit, _ compile.PayloadType, obj *action.ActionRules) action.Action {
	return obj
}

func (res *ResourceCompiler) Decorate(_ *compile.CompileEnv, u *compile.Unit) error {
	if u.Payload == compile.PAYLOAD_SHAREDLIB {
		// Generate minimal resources for DLLs
		u.CompilerOptions.Append("/q")
	}
	return nil
}

func (res *ResourceCompiler) Build(bc utils.BuildContext) error {
	windowsFlags := GetWindowsFlags()
	if _, err := utils.GetBuildableFlags(windowsFlags).Need(bc); err != nil {
		return err
	}

	windowsSDKInstall, err := GetWindowsSDKInstall(bc, windowsFlags.WindowsSDK)
	if err != nil {
		return err
	}

	res.CompilerRules.Executable = windowsSDKInstall.ResourceCompiler
	if err := bc.NeedFile(res.CompilerRules.Executable); err != nil {
		return err
	}

	res.CompilerRules.Environment = internal_io.NewProcessEnvironment()
	res.CompilerRules.Environment.Append("PATH", res.CompilerRules.Executable.Dirname.String(), "%PATH%")

	res.CompilerOptions = base.StringSet{
		"/nologo", // no copyright when compiling
		"/fo%2",   // output file injection
		"%1",      // input file
	}

	return nil
}
func (res *ResourceCompiler) Serialize(ar base.Archive) {
	ar.Serializable(&res.CompilerRules)
}

func (res *ResourceCompiler) checkCompilerInterfaceAtCompileTime() compile.Compiler { return res }

func GetWindowsResourceCompiler() utils.BuildFactoryTyped[*ResourceCompiler] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (ResourceCompiler, error) {
		rc := ResourceCompiler{
			CompilerRules: compile.NewCompilerRules(compile.NewCompilerAlias("custom", "rc", "windows_sdk")),
		}
		rc.checkCompilerInterfaceAtCompileTime()
		return rc, nil
	})
}
