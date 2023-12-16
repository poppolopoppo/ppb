package windows

import (
	"fmt"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

type ResourceCompiler struct {
	CompilerRules
}

func (res *ResourceCompiler) GetCompiler() *CompilerRules { return &res.CompilerRules }

func (res *ResourceCompiler) Extname(PayloadType) string {
	return ".res"
}

func (res *ResourceCompiler) CppRtti(*Facet, bool)      {}
func (res *ResourceCompiler) CppStd(*Facet, CppStdType) {}

func (res *ResourceCompiler) DebugSymbols(*Unit) {}

func (res *ResourceCompiler) AllowCaching(*Unit, PayloadType) CacheModeType     { return CACHE_NONE }
func (res *ResourceCompiler) AllowDistribution(*Unit, PayloadType) DistModeType { return DIST_NONE }
func (res *ResourceCompiler) AllowResponseFile(*Unit, PayloadType) CompilerSupportType {
	return COMPILERSUPPORT_UNSUPPORTED
}
func (res *ResourceCompiler) AllowEditAndContinue(*Unit, PayloadType) CompilerSupportType {
	return COMPILERSUPPORT_UNSUPPORTED
}

func (res *ResourceCompiler) Define(facet *Facet, def ...string) {
	for _, x := range def {
		facet.AddCompilationFlag(fmt.Sprintf("/d%s", x))
	}
}
func (res *ResourceCompiler) Link(*Facet, LinkType) {}
func (res *ResourceCompiler) PrecompiledHeader(*Unit) {
}
func (res *ResourceCompiler) Sanitizer(*Facet, SanitizerType) {}

func (res *ResourceCompiler) ForceInclude(*Facet, ...Filename) {}
func (res *ResourceCompiler) IncludePath(facet *Facet, dirs ...Directory) {
	for _, x := range dirs {
		facet.AddCompilationFlag(fmt.Sprintf("/i%v", x))
	}
}
func (res *ResourceCompiler) ExternIncludePath(facet *Facet, dirs ...Directory) {
	res.IncludePath(facet, dirs...)
}
func (res *ResourceCompiler) SystemIncludePath(facet *Facet, dirs ...Directory) {
	res.IncludePath(facet, dirs...)
}
func (res *ResourceCompiler) Library(*Facet, ...Filename)      {}
func (res *ResourceCompiler) LibraryPath(*Facet, ...Directory) {}
func (res *ResourceCompiler) SourceDependencies(obj *ActionRules) Action {
	return obj
}

func (res *ResourceCompiler) Decorate(_ *CompileEnv, u *Unit) error {
	if u.Payload == PAYLOAD_SHAREDLIB {
		// Generate minimal resources for DLLs
		u.CompilerOptions.Append("/q")
	}
	return nil
}

func (res *ResourceCompiler) Build(bc BuildContext) error {
	windowsFlags := GetWindowsFlags()
	if _, err := GetBuildableFlags(windowsFlags).Need(bc); err != nil {
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

	res.CompilerRules.Environment = NewProcessEnvironment()
	res.CompilerRules.Environment.Append("PATH", res.CompilerRules.Executable.Dirname.String(), "%PATH%")

	res.CompilerOptions = StringSet{
		"/nologo", // no copyright when compiling
		"/fo%2",   // output file injection
		"%1",      // input file
	}

	return nil
}
func (res *ResourceCompiler) Serialize(ar Archive) {
	ar.Serializable(&res.CompilerRules)
}

func (res *ResourceCompiler) checkCompilerInterfaceAtCompileTime() Compiler { return res }

func GetWindowsResourceCompiler() BuildFactoryTyped[*ResourceCompiler] {
	return MakeBuildFactory(func(bi BuildInitializer) (ResourceCompiler, error) {
		rc := ResourceCompiler{
			CompilerRules: NewCompilerRules(NewCompilerAlias("custom", "rc", "windows_sdk")),
		}
		rc.checkCompilerInterfaceAtCompileTime()
		return rc, nil
	})
}
