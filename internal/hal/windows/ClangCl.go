package windows

import (
	"fmt"
	"os/exec"
	"regexp"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/internal/hal/generic"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
)

/***************************************
 * LLVM for Windows
 ***************************************/

type LlvmProductInstall struct {
	Version     string
	InstallDir  Directory
	ClangCl_exe Filename
	LlvmLib_exe Filename
	LldLink_exe Filename
}

type ClangCompiler struct {
	LlvmProductInstall BuildAlias
	UseMsvcLibrarian   bool
	UseMsvcLinker      bool
	MsvcCompiler
}

func (clang *ClangCompiler) GetLlvmProduct() (*LlvmProductInstall, error) {
	return FindGlobalBuildable[*LlvmProductInstall](clang.LlvmProductInstall)
}

/***************************************
 * Compiler interface (override MsvcCompiler)
 ***************************************/

func (clang *ClangCompiler) Extname(x PayloadType) string {
	switch x {
	case PAYLOAD_DEPENDENCIES:
		return ".obj.d"
	default:
		return clang.MsvcCompiler.Extname(x)
	}
}
func (clang *ClangCompiler) ExternIncludePath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("/imsvc" + x.String())
	}
}
func (clang *ClangCompiler) SystemIncludePath(facet *Facet, dirs ...Directory) {
	clang.ExternIncludePath(facet, dirs...)
}
func (clang *ClangCompiler) DebugSymbols(u *Unit) {
	clang.MsvcCompiler.DebugSymbols(u)

	// https://blog.llvm.org/2018/01/improving-link-time-on-windows-with.html
	if !clang.UseMsvcLinker && u.LinkerOptions.RemoveAll("/DEBUG") {
		//f.CompilerOptions.Append("-mllvm", "-emit-codeview-ghash-section")
		u.LinkerOptions.Append("/DEBUG:GHASH")
	}

	// not supported by clang-cl
	u.RemoveCompilationFlag("/Zf")
}
func (clang *ClangCompiler) SourceDependencies(obj *ActionRules) Action {
	result := &GnuSourceDependenciesAction{
		ActionRules: *obj.GetAction(),
	}
	result.GnuDepFile = result.Outputs[0].ReplaceExt(clang.Extname(PAYLOAD_DEPENDENCIES))
	result.Arguments.Append("/clang:--write-dependencies", "/clang:-MF"+MakeLocalFilename(result.GnuDepFile))
	result.Extras.Append(result.GnuDepFile)
	return result
}
func (clang *ClangCompiler) Decorate(compileEnv *CompileEnv, u *Unit) error {
	err := clang.MsvcCompiler.Decorate(compileEnv, u)
	if err != nil {
		return err
	}

	// add platform command flags for clang, intellisense is still assuming a cl-like frontend
	switch compileEnv.GetPlatform().Arch {
	case ARCH_ARM, ARCH_X86:
		u.AddCompilationFlag_NoAnalysis("-m32")
	case ARCH_ARM64, ARCH_X64:
		u.AddCompilationFlag_NoAnalysis("-m64")
	default:
		UnexpectedValue(compileEnv.GetPlatform().Arch)
	}

	if u.CppRules.CompilerVerbose.Get() {
		// enable compiler verbose output
		u.CompilerOptions.AppendUniq("-v")
	}

	if u.Deterministic.Get() {
		// no support for "/pathmap:" in clang-cl atm
		pathMap := clang.MsvcCompiler.GetPathmap()
		u.RemoveCompilationFlag(pathMap)
		// https://blog.llvm.org/2019/11/deterministic-builds-with-clang-and-lld.html#:~:text=fdebug%2Dcompilation%2Ddir%20.
		u.AddCompilationFlag("/clang:-fdebug-compilation-dir=.")
		if !clang.UseMsvcLinker {
			// no support for "/pathmap:" or "/experimental:deterministic" in lld-link atm
			u.LinkerOptions.Remove(pathMap, "/experimental:deterministic")
			// https://blog.llvm.org/2019/11/deterministic-builds-with-clang-and-lld.html#:~:text=understand%20the%20flag-,/lldignoreenv,-flag%2C%20which%20makes
			u.LinkerOptions.Append("/lldignoreenv")
			// https://blog.llvm.org/2019/11/deterministic-builds-with-clang-and-lld.html#:~:text=You%20can%20pass-,/pdbsourcepath%3AX%3A%5Cfake%5Cprefix,-to%20lld%2Dlink
			u.LinkerOptions.Append("/pdbsourcepath:" + UFS.Root.Path)
			// https://blog.llvm.org/2019/11/deterministic-builds-with-clang-and-lld.html#:~:text=also%20offers%20a-,/timestamp%3A,-flag%20that%20you
			deterministicTimestamp := CommandEnv.BuildTime().UTC().Unix()
			u.LinkerOptions.Append(fmt.Sprintf("/timestamp:%d", deterministicTimestamp))
		}
		if !clang.UseMsvcLibrarian {
			// no support for "/Brepro" or "/experimental:deterministic" in llvm-lib atm
			u.LibrarianOptions.Remove("/Brepro", "/experimental:deterministic")
		}
	}

	// flags added by msvc but not supported by clang-cl, llvm-lib or lld-link
	u.RemoveCompilationFlag("/WX", "/JMC-")
	if !clang.UseMsvcLibrarian {
		u.LibrarianOptions.Remove("/WX", "/SUBSYSTEM:WINDOWS", "/NODEFAULTLIB")
	}
	if !clang.UseMsvcLinker {
		u.LinkerOptions.Remove("/WX", "/LTCG", "/LTCG:INCREMENTAL", "/LTCG:OFF", "/NODEFAULTLIB", "/d2:-cgsummary")
	}

	// #TODO: wait for MSTL/llvm to be fixed with this optimization
	// if u.Payload == PAYLOAD_SHAREDLIB {
	// 	// https://blog.llvm.org/2018/11/30-faster-windows-builds-with-clang-cl_14.html
	// 	u.CompilerOptions.Append("/Zc:dllexportInlines-") // not workig with /MD and std
	// 	u.PrecompiledHeaderOptions.Append("/Zc:dllexportInlines-")
	// }

	return nil
}
func (clang *ClangCompiler) Serialize(ar Archive) {
	ar.Serializable(&clang.LlvmProductInstall)
	ar.Bool(&clang.UseMsvcLibrarian)
	ar.Bool(&clang.UseMsvcLinker)
	ar.Serializable(&clang.MsvcCompiler)
}

func (clang *ClangCompiler) Build(bc BuildContext) error {
	llvm, err := GetLlvmProductInstall().Need(bc)
	if err != nil {
		return err
	}

	clang.LlvmProductInstall = llvm.Alias()

	if msvc, err := GetMsvcCompiler(clang.Arch).Need(bc); err == nil {
		compilerAlias := clang.CompilerAlias
		clang.MsvcCompiler = *msvc
		clang.CompilerAlias = compilerAlias
	} else {
		return err
	}

	compileFlags := GetCompileFlags()
	windowsFlags := GetWindowsFlags()

	clang.UseMsvcLibrarian = !windowsFlags.LlvmToolchain.Get()
	clang.UseMsvcLinker = !windowsFlags.LlvmToolchain.Get()

	rules := clang.GetCompiler()
	rules.Executable = llvm.ClangCl_exe
	rules.ExtraFiles = FileSet{
		llvm.InstallDir.Folder("bin").File("msvcp140.dll"),
		llvm.InstallDir.Folder("bin").File("vcruntime140.dll"),
	}

	if !clang.UseMsvcLibrarian {
		LogVeryVerbose(LogWindows, "%v: use llvm librarian %q", clang.CompilerAlias, llvm.LlvmLib_exe)
		rules.Librarian = llvm.LlvmLib_exe
	}
	if !clang.UseMsvcLinker {
		LogVeryVerbose(LogWindows, "%v: use llvm linker %q", clang.CompilerAlias, llvm.LldLink_exe)
		rules.Linker = llvm.LldLink_exe
	}

	rules.Defines.Append("CPP_CLANG", "LLVM_FOR_WINDOWS", "_CRT_SECURE_NO_WARNINGS")
	rules.AddCompilationFlag_NoAnalysis(
		// compiler output
		"-msse4.2",
		// msvc compatibility
		"-fmsc-version="+clang.MsvcCompiler.MSC_VER.String(),
		"-fms-compatibility",
		"-fms-extensions",
		// generate full debug infos
		"-fstandalone-debug",
		// error reporting
		"-fcolor-diagnostics",
		"/clang:-fno-elide-type",
		"/clang:-fdiagnostics-show-template-tree",
		"/clang:-fmacro-backtrace-limit=0",
		"/clang:-ftemplate-backtrace-limit=0",
	)

	if compileFlags.Benchmark.Get() {
		// https: //aras-p.info/blog/2019/01/16/time-trace-timeline-flame-chart-profiler-for-Clang/
		rules.CompilerOptions.Append("/clang:-ftime-trace")
	}

	if windowsFlags.Permissive.Get() {
		rules.AddCompilationFlag_NoAnalysis("-Wno-error")
	} else {
		rules.AddCompilationFlag_NoAnalysis(
			"-Werror",
			"-Wno-assume",                        // the argument to '__assume' has side effects that will be discarded
			"-Wno-ignored-pragma-optimize",       // pragma optimize n'est pas supporté
			"-Wno-unused-command-line-argument",  // ignore les options non suportées par CLANG (sinon échoue a cause de /WError)
			"-Wno-ignored-attributes",            // ignore les attributs de classe/fonction non supportées par CLANG (sinon échoue a cause de /WError)
			"-Wno-unknown-pragmas",               // ignore les directives pragma non supportées par CLANG (sinon échoue a cause de /WError)
			"-Wno-unused-local-typedef",          // ignore les typedefs locaux non utilisés (nécessaire pour STATIC_ASSERT(x))
			"-Wno-#pragma-messages",              // don't consider #pragma message as warnings
			"-Wno-unneeded-internal-declaration", // ignore unused internal functions beeing stripped)
		)
	}

	rules.SystemIncludePaths.Append(
		llvm.InstallDir.Folder("include", "clang-c"),
		llvm.InstallDir.Folder("include", "llvm-c"),
		llvm.InstallDir.Folder("lib", "clang", llvm.Version, "include"),
	)
	rules.LibraryPaths.Append(
		llvm.InstallDir.Folder("lib"),
		llvm.InstallDir.Folder("lib", "clang", llvm.Version, "lib", "windows"),
	)

	// https://blog.llvm.org/posts/2021-04-05-constructor-homing-for-debug-info/
	rules.CompilerOptions.Append("-Xclang", "-fuse-ctor-homing")
	rules.PrecompiledHeaderOptions.Append("-Xclang", "-fuse-ctor-homing")

	return nil
}

/***************************************
 * Product install
 ***************************************/

var re_clangClVersion = regexp.MustCompile(`(?m)^clang\s+version\s+([\.\d]+)$`)

func (llvm *LlvmProductInstall) Alias() BuildAlias {
	return MakeBuildAlias("HAL", "Windows", "LLVM", "Latest")
}
func (llvm *LlvmProductInstall) Serialize(ar Archive) {
	ar.String(&llvm.Version)
	ar.Serializable(&llvm.InstallDir)
	ar.Serializable(&llvm.ClangCl_exe)
	ar.Serializable(&llvm.LlvmLib_exe)
	ar.Serializable(&llvm.LldLink_exe)
}
func (llvm *LlvmProductInstall) Build(bc BuildContext) error {
	candidatePaths := DirSet{
		MakeDirectory("C:/Program Files/LLVM"),
		MakeDirectory("C:/Program Files (x86)/LLVM")}

	for _, x := range candidatePaths {
		if x.Exists() {
			llvm.InstallDir = x
			llvm.ClangCl_exe = x.Folder("bin").File("clang-cl.exe")
			llvm.LlvmLib_exe = x.Folder("bin").File("llvm-lib.exe")
			llvm.LldLink_exe = x.Folder("bin").File("lld-link.exe")

			if fullVersion, err := exec.Command(llvm.ClangCl_exe.String(), "--version").Output(); err == nil {
				parsed := re_clangClVersion.FindStringSubmatch(UnsafeStringFromBytes(fullVersion))
				if nil == parsed {
					return fmt.Errorf("failed to parse clang-cl version: %v", fullVersion)
				}
				llvm.Version = parsed[1]
			} else {
				return err
			}

			LogTrace(LogWindows, "using LLVM v%s for Windows found in '%v'", llvm.Version, llvm.InstallDir)
			if err := bc.NeedFile(llvm.ClangCl_exe, llvm.LlvmLib_exe, llvm.LldLink_exe); err != nil {
				return err
			}

			return nil
		}
	}

	return fmt.Errorf("can't find LLVM for Windows install path")
}

func GetLlvmProductInstall() BuildFactoryTyped[*LlvmProductInstall] {
	return MakeBuildFactory(func(bi BuildInitializer) (LlvmProductInstall, error) {
		return LlvmProductInstall{}, nil
	})
}

func GetClangCompiler(arch ArchType) BuildFactoryTyped[*ClangCompiler] {
	return MakeBuildFactory(func(bi BuildInitializer) (ClangCompiler, error) {
		return ClangCompiler{
				UseMsvcLibrarian: false,
				UseMsvcLinker:    false,
				MsvcCompiler: MsvcCompiler{
					Arch:          arch,
					CompilerRules: NewCompilerRules(NewCompilerAlias("clang-cl", "LLVM", arch.String())),
				},
			}, bi.NeedFactories(
				GetBuildableFlags(GetCompileFlags()),
				GetBuildableFlags(GetWindowsFlags()))
	})
}
