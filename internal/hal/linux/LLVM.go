//go:build linux

package linux

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	. "github.com/poppolopoppo/ppb/compile"
	. "github.com/poppolopoppo/ppb/internal/hal/generic"
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * LLVM Compiler
 ***************************************/

type LlvmCompiler struct {
	Arch    ArchType
	Version LlvmVersion

	CompilerRules
	ProductInstall *LlvmProductInstall
}

func (llvm *LlvmCompiler) GetCompiler() *CompilerRules { return &llvm.CompilerRules }

func (llvm *LlvmCompiler) Serialize(ar base.Archive) {
	ar.Serializable(&llvm.Arch)
	ar.Serializable(&llvm.Version)
	ar.Serializable(&llvm.CompilerRules)
	base.SerializeExternal(ar, &llvm.ProductInstall)
}

func (llvm *LlvmCompiler) Extname(x PayloadType) string {
	switch x {
	case PAYLOAD_EXECUTABLE:
		return ".out"
	case PAYLOAD_OBJECTLIST:
		return ".o"
	case PAYLOAD_STATICLIB:
		return ".a"
	case PAYLOAD_SHAREDLIB:
		return ".so"
	case PAYLOAD_HEADERUNIT:
		return ".ifc"
	case PAYLOAD_PRECOMPILEDHEADER:
		return ".pch"
	case PAYLOAD_HEADERS:
		return ".h"
	case PAYLOAD_SOURCES:
		return ".cpp"
	case PAYLOAD_DEPENDENCIES:
		return ".d"
	case PAYLOAD_PRECOMPILEDOBJECT:
		base.UnreachableCode()
		return ""
	default:
		base.UnexpectedValue(x)
		return ""
	}
}

func (llvm *LlvmCompiler) AllowCaching(u *Unit, payload PayloadType) action.CacheModeType {
	// #TODO: support deterministic builds with LLVM
	// https://reproducible-builds.org/
	return action.CACHE_NONE
}
func (llvm *LlvmCompiler) AllowDistribution(u *Unit, payload PayloadType) action.DistModeType {
	// #TODO: support IO detouring on Linux
	return action.DIST_NONE
}
func (llvm *LlvmCompiler) AllowResponseFile(u *Unit, payload PayloadType) SupportType {
	// #TODO: support response files equivalent on Linux?
	return SUPPORT_UNAVAILABLE
}
func (msvc *LlvmCompiler) AllowEditAndContinue(u *Unit, payload PayloadType) (result SupportType) {
	return SUPPORT_UNAVAILABLE
}
func (llvm *LlvmCompiler) CppRtti(f *Facet, enabled bool) {
	if enabled {
		f.AddCompilationFlag("-frtti")
	} else {
		f.AddCompilationFlag("-fno-rtti")
	}
}
func (llvm *LlvmCompiler) CppStd(f *Facet, std CppStdType) {
	maxSupported := getCppStdFromLlvm(llvm.Version)
	if int32(std) > int32(maxSupported) {
		std = maxSupported
	}
	switch std {
	case CPPSTD_LATEST, CPPSTD_20:
		f.AddCompilationFlag("-std=c++20")
	case CPPSTD_17:
		f.AddCompilationFlag("-std=c++17")
	case CPPSTD_14:
		f.AddCompilationFlag("-std=c++14")
	case CPPSTD_11:
		f.AddCompilationFlag("-std=c++11")
	default:
		base.UnexpectedValue(std)
	}
}
func (llvm *LlvmCompiler) Define(f *Facet, def ...string) {
	for _, x := range def {
		f.AddCompilationFlag(fmt.Sprint("-D", x))
	}
}
func (llvm *LlvmCompiler) DebugSymbols(u *Unit) {
	switch u.DebugInfo {
	case DEBUGINFO_DISABLED:
		return
	case DEBUGINFO_SYMBOLS:
		base.LogVeryVerbose(LogLinux, "not available on linux: DEBUG_SYMBOLS")
	case DEBUGINFO_HOTRELOAD:
		base.LogVeryVerbose(LogLinux, "not available on linux: DEBUG_HOTRELOAD")
	case DEBUGINFO_EMBEDDED:
	default:
		base.UnexpectedValue(u.DebugInfo)
	}

	u.CompilerOptions.Append("-g") // embedded debug info
}
func (llvm *LlvmCompiler) Link(f *Facet, lnk LinkType) {
	switch lnk {
	case LINK_STATIC:
		return // nothing to do
	case LINK_DYNAMIC:
		f.LinkerOptions.Append("-shared")
	default:
		base.UnexpectedValue(lnk)
	}
}
func (llvm *LlvmCompiler) PrecompiledHeader(u *Unit) {
	switch u.PCH {
	case PCH_HEADERUNIT:
		base.LogWarning(LogLinux, "%v: clang does not support header units with automatic translation, fallback to PCH", u)
		fallthrough
	case PCH_MONOLITHIC, PCH_SHARED:
		u.CompilerOptions.Append(
			"-include"+u.PrecompiledHeader.Relative(UFS.Source),
			"-include-pch", MakeLocalFilename(u.PrecompiledObject))
		if u.PCH != PCH_SHARED {
			u.PrecompiledHeaderOptions.Prepend("-emit-pch", "-xc++-header")
		}
	case PCH_DISABLED:
	default:
		base.UnexpectedValue(u.PCH)
	}
}
func (llvm *LlvmCompiler) Sanitizer(f *Facet, sanitizer SanitizerType) {
	switch sanitizer {
	case SANITIZER_NONE:
		return
	case SANITIZER_ADDRESS:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=address")
	case SANITIZER_THREAD:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=thread")
	case SANITIZER_UNDEFINED_BEHAVIOR:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=ub")
	default:
		base.UnexpectedValue(sanitizer)
	}
	f.Defines.Append("USE_PPE_SANITIZER=1")
}

func (llvm *LlvmCompiler) ForceInclude(f *Facet, inc ...Filename) {
	for _, x := range inc {
		f.AddCompilationFlag_NoAnalysis("-include" + x.Relative(UFS.Source))
	}
}
func (llvm *LlvmCompiler) IncludePath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-I" + MakeLocalDirectory(x))
	}
}
func (llvm *LlvmCompiler) ExternIncludePath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-iframework" + MakeLocalDirectory(x))
	}
}
func (llvm *LlvmCompiler) SystemIncludePath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-isystem" + MakeLocalDirectory(x))
	}
}
func (llvm *LlvmCompiler) Library(f *Facet, lib ...Filename) {
	for _, x := range lib {
		s := MakeLocalFilename(x)
		f.LibrarianOptions.Append(s)
		f.LinkerOptions.Append(s)
	}
}
func (llvm *LlvmCompiler) LibraryPath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		s := x.String()
		llvm.AddCompilationFlag_NoAnalysis("-L" + s)
		f.LinkerOptions.Append("-I" + s)
	}
}
func (llvm *LlvmCompiler) GetPayloadOutput(u *compile.Unit, payload compile.PayloadType, file Filename) Filename {
	if payload == PAYLOAD_PRECOMPILEDOBJECT {
		return file // clang does not output a compiled object when emitting PCH, only a pre-parsed AST
	}
	return file.ReplaceExt(llvm.Extname(payload))
}
func (llvm *LlvmCompiler) CreateAction(u *Unit, _ PayloadType, model *action.ActionModel) action.Action {
	result := &GnuSourceDependenciesAction{
		ActionRules: model.CreateActionRules(),
		GnuDepFile:  model.ExportFile.ReplaceExt(llvm.Extname(PAYLOAD_DEPENDENCIES)),
	}

	allowRelativePath := result.Options.Has(action.OPT_ALLOW_RELATIVEPATH)

	result.Arguments.Append("--write-dependencies", "-MF"+MakeLocalFilenameIFP(result.GnuDepFile, allowRelativePath))
	result.OutputFiles.Append(result.GnuDepFile)
	return result
}
func (llvm *LlvmCompiler) Decorate(compileEnv *CompileEnv, u *Unit) error {
	if u.CompilerVerbose.Get() {
		u.CompilerOptions.AppendUniq("/v")
	}

	switch compileEnv.GetPlatform().Arch {
	case ARCH_X86:
		u.AddCompilationFlag_NoAnalysis("-m32")
	case ARCH_X64:
		u.AddCompilationFlag_NoAnalysis("-m64")
	default:
		base.UnexpectedValue(compileEnv.GetPlatform().Arch)
	}

	// set compiler options from configuration
	switch u.RuntimeLib {
	case RUNTIMELIB_DYNAMIC, RUNTIMELIB_DYNAMIC_DEBUG, RUNTIMELIB_INHERIT:
	case RUNTIMELIB_STATIC, RUNTIMELIB_STATIC_DEBUG:
		u.AddCompilationFlag("-static", "-lc++abi")
	}

	// https://releases.llvm.org/12.0.0/projects/libcxx/docs/DesignDocs/DebugMode.html
	if u.RuntimeLib.IsDebug() {
		u.Defines.Append("_LIBCPP_DEBUG=1")
	} else {
		u.Defines.Append("_LIBCPP_DEBUG=0")
	}

	switch u.Optimize {
	case OPTIMIZE_NONE:
		u.AddCompilationFlag("-O0", "-fno-PIE")
	case OPTIMIZE_FOR_DEBUG:
		u.AddCompilationFlag("-O1", "-fno-PIE")
	case OPTIMIZE_FOR_SIZE:
		u.AddCompilationFlag("-O2", "-fno-PIE", "-ffast-math")
	case OPTIMIZE_FOR_SPEED, OPTIMIZE_FOR_SHIPPING:
		u.AddCompilationFlag("-O3", "-fPIE", "-ffast-math")
	}

	// can only enable LTCG when optimizations are enabled
	if u.Optimize.IsEnabled() {
		llvm_CXX_linkTimeCodeGeneration(u, u.LTO.IsEnabled(), u.Incremental.IsEnabled())
	}

	// runtime security checks
	llvm_CXX_runtimeChecks(u, u.RuntimeChecks.IsEnabled(), !u.Optimize.IsEnabled())

	return nil
}

/***************************************
 * Compiler options per configuration
 ***************************************/

func llvm_CXX_linkTimeCodeGeneration(u *Unit, enabled bool, incremental bool) {
	if enabled {
		u.LibrarianOptions.Append("-T")
		if incremental {
			base.LogVeryVerbose(LogLinux, "%v: using llvm thin link time optimization with caching", u)
			u.CompilerOptions.Append("-flto=thin")
			u.LinkerOptions.Append("-Wl,--thinlto-cache-dir=" + UFS.Transient.AbsoluteFolder("ThinLTO").String())
		} else {
			base.LogVeryVerbose(LogLinux, "%v: using llvm link time optimization", u)
			u.CompilerOptions.Append("-flto")
		}
	} else {
		base.LogVeryVerbose(LogLinux, "%v: disable llvm link time optimization", u)
		u.CompilerOptions.Append("-fno-lto")
	}
}
func llvm_CXX_runtimeChecks(u *Unit, enabled bool, strong bool) {
	if enabled {
		if strong {
			base.LogVeryVerbose(LogLinux, "%v: using llvm strong stack protector", u)
			u.AddCompilationFlag_NoPreprocessor("-fstack-protector-strong")
		} else {
			base.LogVeryVerbose(LogLinux, "%v: using llvm stack protector", u)
			u.AddCompilationFlag_NoPreprocessor("-fstack-protector")
		}
	} else {
		base.LogVeryVerbose(LogLinux, "%v: disable llvm stack protector", u)
		u.AddCompilationFlag_NoPreprocessor("-fno-stack-protector")
	}
}

/***************************************
 * Compiler detection
 ***************************************/

type LlvmProductInstall struct {
	Arch      string
	WantedVer LlvmVersion

	ActualVer     LlvmVersion
	InstallDir    Directory
	Ar            Filename
	Clang         Filename
	ClangPlusPlus Filename
	Llvm_Config   Filename
}

func (x *LlvmProductInstall) Alias() BuildAlias {
	return MakeBuildAlias("HAL", "Linux", "LLVM", x.WantedVer.String(), x.Arch)
}
func (x *LlvmProductInstall) Serialize(ar base.Archive) {
	ar.String(&x.Arch)
	ar.Serializable(&x.WantedVer)

	ar.Serializable(&x.ActualVer)
	ar.Serializable(&x.InstallDir)
	ar.Serializable(&x.Ar)
	ar.Serializable(&x.Clang)
	ar.Serializable(&x.ClangPlusPlus)
	ar.Serializable(&x.Llvm_Config)
}
func (x *LlvmProductInstall) Build(bc BuildContext) error {
	buildCompilerVer := func(suffix string) error {
		base.LogDebug(LogLinux, "llvm: looking for clang++%s...", suffix)
		c := exec.Command("/bin/sh", "-c", "which clang++"+suffix)
		if outp, err := c.Output(); err == nil {
			x.ClangPlusPlus = MakeFilename(strings.TrimSpace(base.UnsafeStringFromBytes(outp)))
		} else {
			return err
		}

		c = exec.Command("/bin/sh", "-c", "realpath $(which clang++"+suffix+")")
		if outp, err := c.Output(); err == nil {
			x.Clang = MakeFilename(strings.TrimSpace(base.UnsafeStringFromBytes(outp)))
		} else {
			return err
		}

		bin := x.Clang.Dirname
		x.InstallDir = bin.Parent()

		x.Ar = bin.File("llvm-ar")
		if _, err := x.Ar.Info(); err != nil {
			return err
		}

		x.Llvm_Config = bin.File("llvm-config")
		if _, err := x.Llvm_Config.Info(); err != nil {
			return err
		}

		c = exec.Command(x.Llvm_Config.String(), "--version")
		if outp, err := c.Output(); err == nil {
			parsed := strings.TrimSpace(base.UnsafeStringFromBytes(outp))
			if n := strings.IndexByte(parsed, '.'); n != -1 {
				parsed = parsed[:n]
			}
			if err = x.ActualVer.Set(parsed); err != nil {
				return err
			}
		} else {
			return err
		}

		if err := bc.NeedDirectories(x.InstallDir); err != nil {
			return err
		}
		if err := bc.NeedFiles(x.Ar, x.Clang, x.ClangPlusPlus, x.Llvm_Config); err != nil {
			return err
		}

		return nil
	}

	var err error
	switch x.WantedVer {
	case LLVM_LATEST:
		for _, actualVer := range GetLlvmVersions() {
			if err = buildCompilerVer("-" + actualVer.String()); err == nil {
				break
			}
		}
	case llvm_any:
		err = buildCompilerVer("" /* no suffix */)
	default:
		err = buildCompilerVer("-" + x.WantedVer.String())
	}

	return err
}

func (llvm *LlvmCompiler) Build(bc BuildContext) error {
	linuxFlags, err := GetLinuxFlags(bc)
	if err != nil {
		return err
	}

	llvm.ProductInstall, err = GetLlvmProductInstall(LlvmProductVer{
		Arch:    llvm.Arch,
		LlvmVer: linuxFlags.LlvmVer,
	}).Need(bc)
	if err != nil {
		return err
	}

	_, err = bc.NeedBuildable(llvm.ProductInstall)
	if err != nil {
		return err
	}

	llvm.Version = llvm.ProductInstall.ActualVer
	llvm.CompilerRules.CppStd = CPPSTD_17
	llvm.CompilerRules.Features = base.MakeEnumSet(
		COMPILER_ALLOW_CACHING,
		COMPILER_ALLOW_DISTRIBUTION,
		COMPILER_ALLOW_SOURCEMAPPING)

	llvm.CompilerRules.Executable = llvm.ProductInstall.ClangPlusPlus
	llvm.CompilerRules.Librarian = llvm.ProductInstall.Ar
	llvm.CompilerRules.Linker = llvm.ProductInstall.Clang

	llvm.CompilerRules.Environment = internal_io.NewProcessEnvironment()
	llvm.CompilerRules.Facet = NewFacet()
	facet := &llvm.CompilerRules.Facet

	facet.Defines.Append(
		"CPP_CLANG",
		"LLVM_FOR_POSIX",
	)

	facet.AddCompilationFlag_NoAnalysis(
		"-Wall", "-Wextra", "-Werror", "-Wfatal-errors",
		"-Wshadow",
		"-Wno-unused-command-line-argument", // #TODO: unsilence this warning
		"-fcolor-diagnostics",
		"-march=native",
		"-mavx", "-msse4.2",
		"-mlzcnt", "-mpopcnt",
		"-fsized-deallocation", // https://isocpp.org/files/papers/n3778.html
		"-c",                   // compile only
		"-o", "%2", "%1",       // input file injection
	)

	if compileFlags, err := GetCompileFlags(bc); err == nil && compileFlags.Benchmark.Get() {
		// https: //aras-p.info/blog/2019/01/16/time-trace-timeline-flame-chart-profiler-for-Clang/
		facet.CompilerOptions.Append("-ftime-trace")
	} else if err != nil {
		return err
	}

	facet.LibrarianOptions.Append("rcs", "%2", "%1")
	facet.LinkerOptions.Append("%1", "-o", "%2")

	if int(llvm.Version) >= int(LLVM_14) {
		facet.AddCompilationFlag_NoPreprocessor("-Xclang -fuse-ctor-homing")
	}

	switch linuxFlags.DumpRecordLayouts {
	case DUMPRECORDLAYOUTS_NONE:
	case DUMPRECORDLAYOUTS_SIMPLE:
		facet.CompilerOptions.Append("-Xclang -fdump-record-layouts-simple")
	case DUMPRECORDLAYOUTS_FULL:
		facet.CompilerOptions.Append("-Xclang -fdump-record-layouts")
	default:
		base.UnexpectedValue(linuxFlags.DumpRecordLayouts)
	}

	return nil
}

type LlvmProductVer struct {
	Arch    ArchType
	LlvmVer LlvmVersion
}

func GetLlvmProductInstall(productVer LlvmProductVer) BuildFactoryTyped[*LlvmProductInstall] {
	return MakeBuildFactory(func(bi BuildInitializer) (LlvmProductInstall, error) {
		return LlvmProductInstall{
			Arch:      productVer.Arch.String(),
			WantedVer: productVer.LlvmVer,
		}, nil
	})
}

func GetLlvmCompiler(arch ArchType) BuildFactoryTyped[*LlvmCompiler] {
	return MakeBuildFactory(func(bi BuildInitializer) (LlvmCompiler, error) {
		llvm := LlvmCompiler{
			Arch:          arch,
			CompilerRules: NewCompilerRules(NewCompilerAlias("clang", "llvm", arch.String())),
		}
		base.Assert(func() bool {
			var compiler compile.Compiler = &llvm
			return compiler == &llvm
		})
		return llvm, nil
	})
}
