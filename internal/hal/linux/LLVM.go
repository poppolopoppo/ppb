//go:build linux

package linux

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/hal/generic"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * LLVM Compiler
 ***************************************/

type LlvmCompiler struct {
	Arch    compile.ArchType
	Version LlvmVersion

	compile.CompilerRules
	ProductInstall *LlvmProductInstall
}

func (llvm *LlvmCompiler) GetCompiler() *compile.CompilerRules { return &llvm.CompilerRules }

func (llvm *LlvmCompiler) Serialize(ar base.Archive) {
	ar.Serializable(&llvm.Arch)
	ar.Serializable(&llvm.Version)
	ar.Serializable(&llvm.CompilerRules)
	base.SerializeExternal(ar, &llvm.ProductInstall)
}

func (llvm *LlvmCompiler) Extname(x compile.PayloadType) string {
	switch x {
	case compile.PAYLOAD_EXECUTABLE:
		return ".out"
	case compile.PAYLOAD_OBJECTLIST:
		return ".o"
	case compile.PAYLOAD_STATICLIB:
		return ".a"
	case compile.PAYLOAD_SHAREDLIB:
		return ".so"
	case compile.PAYLOAD_HEADERUNIT:
		return ".ifc"
	case compile.PAYLOAD_PRECOMPILEDHEADER:
		return ".pch"
	case compile.PAYLOAD_HEADERS:
		return ".h"
	case compile.PAYLOAD_SOURCES:
		return ".cpp"
	case compile.PAYLOAD_DEPENDENCIES:
		return ".d"
	case compile.PAYLOAD_PRECOMPILEDOBJECT:
		// clang does not emit a precompiled header object, only a precompiled header containner a pre-parsed AST
		base.UnreachableCode()
		return ""
	default:
		base.UnexpectedValue(x)
		return ""
	}
}

func (llvm *LlvmCompiler) AllowCaching(u *compile.Unit, payload compile.PayloadType) action.CacheModeType {
	// #TODO: support deterministic builds with LLVM
	// https://reproducible-builds.org/
	return action.CACHE_NONE
}
func (llvm *LlvmCompiler) AllowDistribution(u *compile.Unit, payload compile.PayloadType) action.DistModeType {
	// #TODO: support IO detouring on Linux
	return action.DIST_NONE
}
func (llvm *LlvmCompiler) AllowResponseFile(u *compile.Unit, payload compile.PayloadType) compile.SupportType {
	// #TODO: support response files equivalent on Linux?
	return compile.SUPPORT_UNAVAILABLE
}
func (msvc *LlvmCompiler) AllowEditAndContinue(u *compile.Unit, payload compile.PayloadType) (result compile.SupportType) {
	return compile.SUPPORT_UNAVAILABLE
}
func (llvm *LlvmCompiler) CppRtti(f *compile.Facet, enabled bool) {
	if enabled {
		f.AddCompilationFlag("-frtti")
	} else {
		f.AddCompilationFlag("-fno-rtti")
	}
}
func (llvm *LlvmCompiler) CppStd(f *compile.Facet, std compile.CppStdType) {
	maxSupported := getCppStdFromLlvm(llvm.Version)
	if int32(std) > int32(maxSupported) {
		std = maxSupported
	}
	switch std {
	case compile.CPPSTD_LATEST:
		f.AddCompilationFlag("-std=c++2c")
	case compile.CPPSTD_23:
		f.AddCompilationFlag("-std=c++23")
	case compile.CPPSTD_20:
		f.AddCompilationFlag("-std=c++20")
	case compile.CPPSTD_17:
		f.AddCompilationFlag("-std=c++17")
	case compile.CPPSTD_14:
		f.AddCompilationFlag("-std=c++14")
	case compile.CPPSTD_11:
		f.AddCompilationFlag("-std=c++11")
	default:
		base.UnexpectedValue(std)
	}
}
func (llvm *LlvmCompiler) Define(f *compile.Facet, def ...string) {
	for _, x := range def {
		f.AddCompilationFlag(fmt.Sprint("-D", x))
	}
}
func (llvm *LlvmCompiler) DebugSymbols(u *compile.Unit) {
	switch u.DebugInfo {
	case compile.DEBUGINFO_DISABLED:
		return
	case compile.DEBUGINFO_SYMBOLS:
		base.LogVeryVerbose(LogLinux, "%v: not available when using llvm: DEBUG_SYMBOLS", u)
	case compile.DEBUGINFO_HOTRELOAD:
		base.LogVeryVerbose(LogLinux, "%v: not available when using llvm: DEBUG_HOTRELOAD", u)
	case compile.DEBUGINFO_EMBEDDED:
		u.CompilerOptions.Append("-g") // embedded debug infoj
	default:
		base.UnexpectedValue(u.DebugInfo)
	}
}
func (llvm *LlvmCompiler) Link(f *compile.Facet, lnk compile.LinkType) {
	switch lnk {
	case compile.LINK_STATIC:
		return // nothing to do
	case compile.LINK_DYNAMIC:
		f.LinkerOptions.Append("-shared")
	default:
		base.UnexpectedValue(lnk)
	}
}
func (llvm *LlvmCompiler) PrecompiledHeader(u *compile.Unit) {
	switch u.PCH {
	case compile.PCH_HEADERUNIT:
		base.LogWarning(LogLinux, "%v: clang does not support header units with automatic translation, fallback to PCH", u)
		fallthrough
	case compile.PCH_MONOLITHIC, compile.PCH_SHARED:
		u.CompilerOptions.Append(
			"-include"+u.PrecompiledHeader.Relative(utils.UFS.Source),
			"-include-pch", utils.MakeLocalFilename(u.PrecompiledObject))
		if u.PCH != compile.PCH_SHARED {
			u.PrecompiledHeaderOptions.Prepend(
				"-xc++-header",
				"-fpch-instantiate-templates",
				"-fpch-validate-input-files-content")
		}
	case compile.PCH_DISABLED:
	default:
		base.UnexpectedValue(u.PCH)
	}
}
func (llvm *LlvmCompiler) Sanitizer(f *compile.Facet, sanitizer compile.SanitizerType) {
	switch sanitizer {
	case compile.SANITIZER_NONE:
		return
	case compile.SANITIZER_ADDRESS:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=address")
	case compile.SANITIZER_THREAD:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=thread")
	case compile.SANITIZER_UNDEFINED_BEHAVIOR:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=ub")
	default:
		base.UnexpectedValue(sanitizer)
	}
	f.Defines.Append("USE_PPE_SANITIZER=1")
}

func (llvm *LlvmCompiler) ForceInclude(f *compile.Facet, inc ...utils.Filename) {
	for _, x := range inc {
		f.AddCompilationFlag_NoAnalysis("-include" + x.Relative(utils.UFS.Source))
	}
}
func (llvm *LlvmCompiler) IncludePath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-I" + utils.MakeLocalDirectory(x))
	}
}
func (llvm *LlvmCompiler) ExternIncludePath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-iframework" + utils.MakeLocalDirectory(x))
	}
}
func (llvm *LlvmCompiler) SystemIncludePath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-isystem" + utils.MakeLocalDirectory(x))
	}
}
func (llvm *LlvmCompiler) Library(f *compile.Facet, lib ...string) {
	for _, s := range lib {
		f.LinkerOptions.Append("-l" + s)
	}
}
func (llvm *LlvmCompiler) LibraryPath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		s := x.String()
		f.AddCompilationFlag_NoAnalysis("-L" + s)
		f.LinkerOptions.Append("-I" + s)
	}
}

func (llvm *LlvmCompiler) GetPayloadOutput(u *compile.Unit, payload compile.PayloadType, file utils.Filename) utils.Filename {
	if payload == compile.PAYLOAD_PRECOMPILEDOBJECT {
		return file // clang does not output a compiled object when emitting PCH, only a pre-parsed AST
	}
	return file.ReplaceExt(llvm.Extname(payload))
}
func (llvm *LlvmCompiler) CreateAction(u *compile.Unit, payload compile.PayloadType, model *action.ActionModel) action.Action {
	switch payload {
	case compile.PAYLOAD_OBJECTLIST, compile.PAYLOAD_PRECOMPILEDHEADER, compile.PAYLOAD_PRECOMPILEDOBJECT, compile.PAYLOAD_HEADERUNIT:
		if model.Options.Has(action.OPT_ALLOW_SOURCEDEPENDENCIES) {
			result := &generic.GnuSourceDependenciesAction{
				ActionRules: model.CreateActionRules(),
				GnuDepFile:  model.ExportFile.ReplaceExt(llvm.Extname(compile.PAYLOAD_DEPENDENCIES)),
			}

			allowRelativePath := result.Options.Has(action.OPT_ALLOW_RELATIVEPATH)

			result.Arguments.Append("--write-dependencies", "-MF"+utils.MakeLocalFilenameIFP(result.GnuDepFile, allowRelativePath))
			result.OutputFiles.Append(result.GnuDepFile)
			return result
		}
		fallthrough

	case compile.PAYLOAD_DEBUGSYMBOLS, compile.PAYLOAD_DEPENDENCIES, compile.PAYLOAD_EXECUTABLE, compile.PAYLOAD_HEADERS, compile.PAYLOAD_SHAREDLIB, compile.PAYLOAD_SOURCES, compile.PAYLOAD_STATICLIB:
		rules := model.CreateActionRules()
		return &rules
	default:
		base.UnreachableCode()
		return nil
	}
}
func (llvm *LlvmCompiler) Decorate(bg utils.BuildGraphReadPort, compileEnv *compile.CompileEnv, u *compile.Unit) error {
	if u.CompilerVerbose.Get() {
		u.CompilerOptions.AppendUniq("-v")
	}
	if u.LinkerVerbose.Get() {
		u.LinkerOptions.AppendUniq("-v")
	}

	switch compileEnv.GetPlatform(bg).Arch {
	case compile.ARCH_X86:
		u.AddCompilationFlag_NoAnalysis("-m32")
	case compile.ARCH_X64:
		u.AddCompilationFlag_NoAnalysis("-m64")
	default:
		base.UnexpectedValue(compileEnv.GetPlatform(bg).Arch)
	}

	// set compiler options from configuration
	switch u.RuntimeLib {
	case compile.RUNTIMELIB_DYNAMIC, compile.RUNTIMELIB_DYNAMIC_DEBUG, compile.RUNTIMELIB_INHERIT:
	case compile.RUNTIMELIB_STATIC, compile.RUNTIMELIB_STATIC_DEBUG:
		u.AddCompilationFlag("-static", "-lc++abi")
	}

	// https://releases.llvm.org/12.0.0/projects/libcxx/docs/DesignDocs/DebugMode.html
	if u.RuntimeLib.IsDebug() {
		u.Defines.Append("_LIBCPP_DEBUG=1")
	} else {
		u.Defines.Append("_LIBCPP_DEBUG=0")
	}

	switch u.Warnings.Default {
	case compile.WARNING_ERROR:
		u.AddCompilationFlag("-Werror", "-Wfatal-errors")
		fallthrough
	case compile.WARNING_WARN:
		u.AddCompilationFlag("-Wall")
		if u.Warnings.Pedantic.IsEnabled() {
			u.AddCompilationFlag("-pedantic", "-Wextra")
		}
	case compile.WARNING_DISABLED:
		u.AddCompilationFlag("-Wno-everything")
	}

	llvm_CXX_setWarningLevel(u, "deprecated-declarations", u.Warnings.Deprecation)
	llvm_CXX_setWarningLevel(u, "pedantic", u.Warnings.Pedantic)
	llvm_CXX_setWarningLevel(u, "shadow", u.Warnings.ShadowVariable)
	llvm_CXX_setWarningLevel(u, "undef", u.Warnings.UndefinedMacro)
	llvm_CXX_setWarningLevel(u, "cast-align", u.Warnings.UnsafeTypeCast)
	llvm_CXX_setWarningLevel(u, "cast-qual", u.Warnings.UnsafeTypeCast)
	llvm_CXX_setWarningLevel(u, "conversion", u.Warnings.UnsafeTypeCast)
	llvm_CXX_setWarningLevel(u, "narrowing", u.Warnings.UnsafeTypeCast)

	switch u.Optimize {
	case compile.OPTIMIZE_NONE:
		u.AddCompilationFlag("-O0")
	case compile.OPTIMIZE_FOR_DEBUG:
		u.AddCompilationFlag("-O1")
	case compile.OPTIMIZE_FOR_SIZE:
		u.AddCompilationFlag("-O2")
	case compile.OPTIMIZE_FOR_SPEED, compile.OPTIMIZE_FOR_SHIPPING:
		u.AddCompilationFlag("-O3", "-Ofast")

		// https://blog.quarkslab.com/clang-hardening-cheat-sheet.html
		if u.Payload == compile.PAYLOAD_SHAREDLIB {
			u.AddCompilationFlag("-fPIC")
		} else {
			u.AddCompilationFlag("-fPIE", "-pie")
		}

		if u.Optimize == compile.OPTIMIZE_FOR_SHIPPING {
			u.AddCompilationFlag("-Wl,-z,now", "-Wl,-z,relr")
		}
	}

	switch u.FloatingPoint {
	case compile.FLOATINGPOINT_FAST:
		u.AddCompilationFlag("-ffp-model=aggressive")
	case compile.FLOATINGPOINT_PRECISE:
		u.AddCompilationFlag("-ffp-model=precise")
	case compile.FLOATINGPOINT_STRICT:
		u.AddCompilationFlag("-ffp-model=strict")
	}

	// check for compile instruction sets support
	if u.Instructions.Has(compile.INSTRUCTIONSET_AVX512) {
		u.AddCompilationFlag("-mavx512f")
	}
	if u.Instructions.Has(compile.INSTRUCTIONSET_AVX2) {
		u.AddCompilationFlag("-mavx2")
	}
	if u.Instructions.Has(compile.INSTRUCTIONSET_AVX) {
		u.AddCompilationFlag("-mavx")
	}
	if u.Instructions.Has(compile.INSTRUCTIONSET_SSE2) {
		u.AddCompilationFlag("-msse2")
	}
	if u.Instructions.Has(compile.INSTRUCTIONSET_SSE3) {
		u.AddCompilationFlag("-msse3")
	}
	if u.Instructions.Has(compile.INSTRUCTIONSET_SSE4_1) {
		u.AddCompilationFlag("-msse4.1")
	}
	if u.Instructions.Has(compile.INSTRUCTIONSET_SSE4_2) || u.Instructions.Has(compile.INSTRUCTIONSET_SSE4_a) {
		u.AddCompilationFlag("-msse4.2")
	}

	// can only enable LTCG when optimizations are enabled
	if u.Optimize.IsEnabled() {
		u.AddCompilationFlag("-mlzcnt", "-mpopcnt")

		llvm_CXX_linkTimeCodeGeneration(u, u.LTO.IsEnabled(), u.Incremental.IsEnabled())
	}

	// runtime security checks
	llvm_CXX_runtimeChecks(u, u.RuntimeChecks.IsEnabled(), !u.Optimize.IsEnabled())

	return nil
}

/***************************************
 * Compiler options per configuration
 ***************************************/

func llvm_CXX_setWarningLevel(u *compile.Unit, warning string, level compile.WarningLevel) {
	switch level {
	case compile.WARNING_ERROR:
		u.AddCompilationFlag("-W"+warning, "-Werror="+warning)
	case compile.WARNING_WARN:
		u.AddCompilationFlag("-W"+warning, "-Wno-error="+warning)
	case compile.WARNING_DISABLED:
		u.AddCompilationFlag("-Wno-" + warning)
	}
}
func llvm_CXX_linkTimeCodeGeneration(u *compile.Unit, enabled bool, incremental bool) {
	if enabled {
		u.LibrarianOptions.Append("-T")
		if incremental {
			base.LogVeryVerbose(LogLinux, "%v: using llvm thin link time optimization with caching", u)
			u.CompilerOptions.Append("-flto=thin")
			u.LinkerOptions.Append("-Wl,--thinlto-cache-dir=" + utils.UFS.Transient.AbsoluteFolder("ThinLTO").String())
		} else {
			base.LogVeryVerbose(LogLinux, "%v: using llvm link time optimization", u)
			u.CompilerOptions.Append("-flto")
		}
	} else {
		base.LogVeryVerbose(LogLinux, "%v: disable llvm link time optimization", u)
		u.CompilerOptions.Append("-fno-lto")
	}
}
func llvm_CXX_runtimeChecks(u *compile.Unit, enabled bool, strong bool) {
	if enabled {
		if strong {
			base.LogVeryVerbose(LogLinux, "%v: using glibc _FORTIFY_SOURCE=2", u)
			u.Defines.Append("_FORTIFY_SOURCE=2")
			base.LogVeryVerbose(LogLinux, "%v: using llvm strong stack protector", u)
			u.AddCompilationFlag_NoPreprocessor("-fstack-protector-strong")
		} else {
			base.LogVeryVerbose(LogLinux, "%v: using glibc _FORTIFY_SOURCE=1", u)
			u.Defines.Append("_FORTIFY_SOURCE=1")
			base.LogVeryVerbose(LogLinux, "%v: using llvm stack protector", u)
			u.AddCompilationFlag_NoPreprocessor("-fstack-protector")
		}
	} else {
		base.LogVeryVerbose(LogLinux, "%v: disable llvm stack protector", u)
		u.Defines.Append("_FORTIFY_SOURCE=0")
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
	InstallDir    utils.Directory
	Ar            utils.Filename
	Clang         utils.Filename
	ClangPlusPlus utils.Filename
	Llvm_Config   utils.Filename
}

func (x *LlvmProductInstall) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("HAL", "Linux", "LLVM", x.WantedVer.String(), x.Arch)
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

func (x *LlvmProductInstall) findToolchain(suffix string) (err error) {
	base.LogTrace(LogLinux, "llvm: looking for toolchain compiler 'clang%s'...", suffix)

	if x.Clang, err = utils.UFS.Which("clang" + suffix); err != nil {
		return err
	}
	if x.ClangPlusPlus, err = utils.UFS.Which("clang++" + suffix); err != nil {
		return err
	}
	if x.Ar, err = utils.UFS.Which("llvm-ar" + suffix); err != nil {
		return err
	}
	if x.Llvm_Config, err = utils.UFS.Which("llvm-config" + suffix); err != nil {
		return err
	}
	if realpath, err := x.ClangPlusPlus.Realpath(); err == nil {
		x.InstallDir = realpath.Dirname.Parent()
	} else {
		return err
	}

	if outp, err := exec.Command(x.Llvm_Config.String(), "--version").Output(); err == nil {
		parsed := strings.TrimSpace(base.UnsafeStringFromBytes(outp))
		if n := strings.IndexByte(parsed, '.'); n != -1 {
			parsed = parsed[:n]
		}
		if err = x.ActualVer.Set(parsed); err != nil {
			return err
		}
	} else {
		errMsg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			errMsg = base.UnsafeStringFromBytes(ee.Stderr)
		}
		base.LogError(LogLinux, "llvm: failed to find clang++ version: %s", errMsg)
		return err
	}

	return nil
}
func (x *LlvmProductInstall) Build(bc utils.BuildContext) (err error) {
	switch x.WantedVer {
	case LLVM_LATEST:
		for _, actualVer := range GetLlvmVersions() {
			if err = x.findToolchain("-" + actualVer.String()); err == nil {
				break
			}
		}
	case llvm_any:
		err = x.findToolchain("" /* no suffix */)
	default:
		err = x.findToolchain("-" + x.WantedVer.String())
	}
	if err != nil {
		return err
	}

	bc.Annotate(utils.AnnocateBuildCommentWith(x.ActualVer))

	if err = bc.NeedDirectories(x.InstallDir); err != nil {
		return err
	}
	if err = bc.NeedFiles(x.Ar, x.Clang, x.ClangPlusPlus, x.Llvm_Config); err != nil {
		return err
	}

	return nil
}

func (llvm *LlvmCompiler) Build(bc utils.BuildContext) error {
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
	llvm.CompilerRules.Features = base.NewEnumSet(
		compile.COMPILER_ALLOW_CACHING,
		compile.COMPILER_ALLOW_DISTRIBUTION,
		compile.COMPILER_ALLOW_SOURCEMAPPING)

	llvm.CompilerRules.Executable = llvm.ProductInstall.ClangPlusPlus
	llvm.CompilerRules.Librarian = llvm.ProductInstall.Ar
	llvm.CompilerRules.Linker = llvm.ProductInstall.ClangPlusPlus

	llvm.CompilerRules.Environment = internal_io.NewProcessEnvironment()
	llvm.CompilerRules.Facet = compile.NewFacet()
	facet := &llvm.CompilerRules.Facet

	facet.Defines.Append(
		"CPP_CLANG",
		"CPP_COMPILER=Clang",
		"LLVM_FOR_POSIX",
	)

	facet.AddCompilationFlag_NoAnalysis(
		"-o", "%2", "%1", // input file injection
		"-Wformat", "-Wformat-security", // detect Potential Formatting Attack
		"-Wno-#pragma-messages",             // silence Unity pragma messages, which are interpreted as warnings by clang
		"-Wno-unused-command-line-argument", // #TODO: unsilence this warning (-lxxx are generating warnings when do not directly consumes specified libraries)
		"-fcolor-diagnostics",
		"-mlzcnt", "-mpopcnt", "-mbmi",
		"-fsized-deallocation", // https://isocpp.org/files/papers/n3778.html
		"-c",                   // compile only
	)

	if compileFlags, err := compile.GetCompileFlags(bc); err == nil && compileFlags.Benchmark.Get() {
		// https: //aras-p.info/blog/2019/01/16/time-trace-timeline-flame-chart-profiler-for-Clang/
		facet.CompilerOptions.Append("-ftime-trace")
	} else if err != nil {
		return err
	}

	facet.LibrarianOptions.Append("rcs", "%2", "%1")
	facet.LinkerOptions.Append("-o", "%2", "%1")

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
	Arch    compile.ArchType
	LlvmVer LlvmVersion
}

func GetLlvmProductInstall(productVer LlvmProductVer) utils.BuildFactoryTyped[*LlvmProductInstall] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (LlvmProductInstall, error) {
		return LlvmProductInstall{
			Arch:      productVer.Arch.String(),
			WantedVer: productVer.LlvmVer,
		}, nil
	})
}

func GetLlvmCompiler(arch compile.ArchType) utils.BuildFactoryTyped[*LlvmCompiler] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (LlvmCompiler, error) {
		llvm := LlvmCompiler{
			Arch:          arch,
			CompilerRules: compile.NewCompilerRules(compile.NewCompilerAlias("clang", "llvm", arch.String())),
		}
		base.Assert(func() bool {
			var compiler compile.Compiler = &llvm
			return compiler == &llvm
		})
		return llvm, nil
	})
}
