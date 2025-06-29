package linux

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/internal/hal/generic"
	"github.com/poppolopoppo/ppb/utils"

	internal_io "github.com/poppolopoppo/ppb/internal/io"
)

// *************************************
// * GCC Compiler
// *************************************

type GccCompiler struct {
	Arch    compile.ArchType
	Version GccVersion

	compile.CompilerRules
	ProductInstall *GccProductInstall
}

func (gcc *GccCompiler) GetCompiler() *compile.CompilerRules { return &gcc.CompilerRules }

func (gcc *GccCompiler) Serialize(ar base.Archive) {
	ar.Serializable(&gcc.Arch)
	ar.Serializable(&gcc.Version)
	ar.Serializable(&gcc.CompilerRules)
	base.SerializeExternal(ar, &gcc.ProductInstall)
}

func (gcc *GccCompiler) Extname(x compile.PayloadType) string {
	switch x {
	case compile.PAYLOAD_EXECUTABLE:
		return ""
	case compile.PAYLOAD_OBJECTLIST:
		return ".o"
	case compile.PAYLOAD_STATICLIB:
		return ".a"
	case compile.PAYLOAD_SHAREDLIB:
		return ".so"
	case compile.PAYLOAD_HEADERS:
		return ".h"
	case compile.PAYLOAD_SOURCES:
		return ".cpp"
	case compile.PAYLOAD_DEPENDENCIES:
		return ".d"
	case compile.PAYLOAD_HEADERUNIT:
		return ".ifc"
	case compile.PAYLOAD_PRECOMPILEDHEADER:
		// gcc replaces .h when it detects .h.gch file with the same name
		return ".h.pch"
	case compile.PAYLOAD_PRECOMPILEDOBJECT:
		// gcc does not emit a precompiled header object, only a precompiled header containner a pre-parsed AST
		base.UnreachableCode()
		return ""
	default:
		base.UnexpectedValue(x)
		return ""
	}
}

func (gcc *GccCompiler) AllowCaching(_ *compile.Unit, _ compile.PayloadType) action.CacheModeType {
	// #TODO: support deterministic builds with GCC
	// https://reproducible-builds.org/
	return action.CACHE_NONE
}
func (gcc *GccCompiler) AllowDistribution(_ *compile.Unit, _ compile.PayloadType) action.DistModeType {
	// #TODO: support IO detouring on Linux
	return action.DIST_NONE
}
func (gcc *GccCompiler) AllowResponseFile(_ *compile.Unit, _ compile.PayloadType) compile.SupportType {
	// #TODO: support response files equivalent on Linux?
	return compile.SUPPORT_UNAVAILABLE
}
func (gcc *GccCompiler) AllowEditAndContinue(_ *compile.Unit, _ compile.PayloadType) compile.SupportType {
	return compile.SUPPORT_UNAVAILABLE
}

func (gcc *GccCompiler) CppRtti(f *compile.Facet, enabled bool) {
	if enabled {
		f.AddCompilationFlag("-frtti")
	} else {
		f.AddCompilationFlag("-fno-rtti")
	}
}

func (gcc *GccCompiler) CppStd(f *compile.Facet, std compile.CppStdType) {
	maxSupported := getCppStdFromGcc(gcc.Version)
	if int32(std) > int32(maxSupported) {
		std = maxSupported
	}
	switch std {
	case compile.CPPSTD_LATEST:
		f.AddCompilationFlag("-std=c++2b")
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

func (gcc *GccCompiler) Define(f *compile.Facet, defs ...string) {
	for _, d := range defs {
		f.AddCompilationFlag(fmt.Sprint("-D", d))
	}
}

func (gcc *GccCompiler) DebugSymbols(u *compile.Unit) {
	switch u.DebugInfo {
	case compile.DEBUGINFO_DISABLED:
		return
	case compile.DEBUGINFO_SYMBOLS:
		base.LogVeryVerbose(LogLinux, "%v: not available when using gcc: DEBUG_SYMBOLS", u)
	case compile.DEBUGINFO_HOTRELOAD:
		base.LogVeryVerbose(LogLinux, "%v: not available when using gcc: DEBUG_HOTRELOAD", u)
	case compile.DEBUGINFO_EMBEDDED:
		u.CompilerOptions.Append("-g")
	default:
		base.UnexpectedValue(u.DebugInfo)
	}
}

func (gcc *GccCompiler) Link(f *compile.Facet, lnk compile.LinkType) {
	switch lnk {
	case compile.LINK_STATIC:
		return
	case compile.LINK_DYNAMIC:
		f.LinkerOptions.Append("-shared")
	default:
		base.UnexpectedValue(lnk)
	}
}

func (gcc *GccCompiler) PrecompiledHeader(u *compile.Unit) {
	switch u.PCH {
	case compile.PCH_HEADERUNIT:
		base.LogWarning(LogLinux, "%v: gcc does not support header units with automatic translation, fallback to PCH", u)
		fallthrough
	case compile.PCH_MONOLITHIC, compile.PCH_SHARED:
		u.CompilerOptions.Append("-include", utils.MakeLocalFilename(u.PrecompiledHeader))
		u.IncludePaths.Prepend(u.PrecompiledObject.Dirname)
	case compile.PCH_DISABLED:
	default:
		base.UnexpectedValue(u.PCH)
	}
}
func (gcc *GccCompiler) Sanitizer(f *compile.Facet, sanitizer compile.SanitizerType) {
	switch sanitizer {
	case compile.SANITIZER_NONE:
		return
	case compile.SANITIZER_ADDRESS:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=address")
	case compile.SANITIZER_THREAD:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=thread")
	case compile.SANITIZER_UNDEFINED_BEHAVIOR:
		f.AddCompilationFlag_NoPreprocessor("-fsanitize=undefined")
	default:
		base.UnexpectedValue(sanitizer)
	}
	f.Defines.Append("USE_PPE_SANITIZER=1")
}

func (gcc *GccCompiler) ForceInclude(f *compile.Facet, inc ...utils.Filename) {
	for _, x := range inc {
		f.AddCompilationFlag_NoAnalysis("-include" + x.Relative(utils.UFS.Source))
	}
}
func (gcc *GccCompiler) IncludePath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-I" + utils.MakeLocalDirectory(x))
	}
}
func (gcc *GccCompiler) ExternIncludePath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		// gcc does not support -iframework
		f.AddCompilationFlag_NoAnalysis("-isystem" + utils.MakeLocalDirectory(x))
	}
}
func (gcc *GccCompiler) SystemIncludePath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("-isystem" + utils.MakeLocalDirectory(x))
	}
}
func (gcc *GccCompiler) Library(f *compile.Facet, lib ...string) {
	for _, s := range lib {
		f.LinkerOptions.Append("-l" + s)
	}
}
func (gcc *GccCompiler) LibraryPath(f *compile.Facet, dirs ...utils.Directory) {
	for _, x := range dirs {
		s := x.String()
		f.AddCompilationFlag_NoAnalysis("-L" + s)
		f.LinkerOptions.Append("-I" + s)
	}
}

func (gcc *GccCompiler) GetPayloadOutput(u *compile.Unit, payload compile.PayloadType, file utils.Filename) utils.Filename {
	if payload == compile.PAYLOAD_PRECOMPILEDOBJECT {
		return file // gcc does not output a compiled object when emitting PCH, only a pre-parsed AST
	}
	return file.ReplaceExt(gcc.Extname(payload))
}
func (gcc *GccCompiler) CreateAction(u *compile.Unit, payload compile.PayloadType, model *action.ActionModel) action.Action {
	switch payload {
	case compile.PAYLOAD_OBJECTLIST, compile.PAYLOAD_DEPENDENCIES:
		if model.Options.Has(action.OPT_ALLOW_SOURCEDEPENDENCIES) {
			result := &generic.GnuSourceDependenciesAction{
				ActionRules: model.CreateActionRules(),
				GnuDepFile:  model.ExportFile.ReplaceExt(gcc.Extname(compile.PAYLOAD_DEPENDENCIES)),
			}
			allowRelativePath := result.Options.Has(action.OPT_ALLOW_RELATIVEPATH)
			result.Arguments.Append("--write-dependencies", "-MF"+utils.MakeLocalFilenameIFP(result.GnuDepFile, allowRelativePath))
			result.OutputFiles.Append(result.GnuDepFile)
			return result
		}
		fallthrough
	default:
		rules := model.CreateActionRules()
		return &rules
	}
}

func (gcc *GccCompiler) Decorate(bg utils.BuildGraphReadPort, compileEnv *compile.CompileEnv, u *compile.Unit) error {
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

	// Runtime library selection
	switch u.RuntimeLib {
	case compile.RUNTIMELIB_STATIC, compile.RUNTIMELIB_STATIC_DEBUG:
		u.AddCompilationFlag("-static")
	}

	// Enable libc debugging if necessary
	if u.RuntimeLib.IsDebug() {
		u.Defines.Append(
			"_GLIBCXX_DEBUG",
			"_GLIBCXX_DEBUG_PEDANTIC",
		)
	}

	// Warnings
	switch u.Warnings.Default {
	case compile.WARNING_ERROR:
		u.AddCompilationFlag("-Werror")
		fallthrough
	case compile.WARNING_WARN:
		u.AddCompilationFlag("-Wall")
		if u.Warnings.Pedantic.IsEnabled() {
			u.AddCompilationFlag("-Wextra", "-pedantic")
		}
	case compile.WARNING_DISABLED:
		u.AddCompilationFlag("-w")
	}

	gcc_CXX_setWarningLevel(u, "deprecated-declarations", u.Warnings.Deprecation)
	gcc_CXX_setWarningLevel(u, "null-reference", u.Warnings.Pedantic)
	gcc_CXX_setWarningLevel(u, "pedantic", u.Warnings.Pedantic)
	gcc_CXX_setWarningLevel(u, "shadow", u.Warnings.ShadowVariable)
	gcc_CXX_setWarningLevel(u, "undef", u.Warnings.UndefinedMacro)
	gcc_CXX_setWarningLevel(u, "cast-align", u.Warnings.UnsafeTypeCast)
	gcc_CXX_setWarningLevel(u, "cast-qual", u.Warnings.UnsafeTypeCast)
	gcc_CXX_setWarningLevel(u, "conversion", u.Warnings.UnsafeTypeCast)
	gcc_CXX_setWarningLevel(u, "double-promotion", u.Warnings.UnsafeTypeCast)
	gcc_CXX_setWarningLevel(u, "narrowing", u.Warnings.UnsafeTypeCast)
	gcc_CXX_setWarningLevel(u, "sign-conversion", u.Warnings.UnsafeTypeCast)

	// Static code analyzer
	if u.StaticAnalysis.IsEnabled() {
		u.AddCompilationFlag_NoPreprocessor(
			"-fanalyzer",
		)
	}

	// Optimization
	switch u.Optimize {
	case compile.OPTIMIZE_NONE:
		u.AddCompilationFlag("-O0")
	case compile.OPTIMIZE_FOR_DEBUG:
		u.AddCompilationFlag("-Og")
	case compile.OPTIMIZE_FOR_SIZE:
		u.AddCompilationFlag("-Os")
	case compile.OPTIMIZE_FOR_SPEED, compile.OPTIMIZE_FOR_SHIPPING:
		u.AddCompilationFlag("-O3")
		if u.Payload == compile.PAYLOAD_SHAREDLIB {
			u.AddCompilationFlag("-fPIC")
		} else {
			u.AddCompilationFlag("-fPIE", "-pie")
		}
	}

	switch u.FloatingPoint {
	case compile.FLOATINGPOINT_FAST:
		u.AddCompilationFlag("-ffp-model=fast")
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

	// Link Time Optimization
	if u.LTO.IsEnabled() {
		u.AddCompilationFlag("-flto")
		u.LibrarianOptions.Append("-flto")
		u.LinkerOptions.Append("-flto")
	} else {
		u.AddCompilationFlag("-fno-lto")
	}

	// Runtime security checks
	if u.RuntimeChecks.IsEnabled() {
		if !u.Optimize.IsEnabled() {
			u.Defines.Append("_FORTIFY_SOURCE=2")
			u.AddCompilationFlag_NoPreprocessor("-fstack-protector-strong")
		} else {
			u.Defines.Append("_FORTIFY_SOURCE=1")
			u.AddCompilationFlag_NoPreprocessor("-fstack-protector")
		}
	} else {
		u.Defines.Append("_FORTIFY_SOURCE=0")
		u.AddCompilationFlag_NoPreprocessor("-fno-stack-protector")
	}

	return nil
}

/***************************************
 * Compiler options per configuration
 ***************************************/

func gcc_CXX_setWarningLevel(u *compile.Unit, warning string, level compile.WarningLevel) {
	switch level {
	case compile.WARNING_ERROR:
		u.AddCompilationFlag("-W"+warning, "-Werror="+warning)
	case compile.WARNING_WARN:
		u.AddCompilationFlag("-W"+warning, "-Wno-error="+warning)
	case compile.WARNING_DISABLED:
		u.AddCompilationFlag("-Wno-" + warning)
	}
}

/***************************************
 * GCC product install detection
 ***************************************/

type GccProductInstall struct {
	Arch      string
	WantedVer GccVersion

	ActualVer  GccVersion
	InstallDir utils.Directory
	Gcc        utils.Filename
	Gpp        utils.Filename
	Ar         utils.Filename
}

func (x *GccProductInstall) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("HAL", "Linux", "GCC", x.WantedVer.String(), x.Arch)
}

func (x *GccProductInstall) Serialize(ar base.Archive) {
	ar.String(&x.Arch)
	ar.Serializable(&x.WantedVer)

	ar.Serializable(&x.ActualVer)
	ar.Serializable(&x.InstallDir)
	ar.Serializable(&x.Gcc)
	ar.Serializable(&x.Gpp)
	ar.Serializable(&x.Ar)
}

var re_gccMatchVersion = regexp.MustCompile(`(?m)^gcc\s+(?:\(.*\)\s+)([\d\.]+)\s*$`)

func (x *GccProductInstall) findToolchain(suffix string) (err error) {
	base.LogTrace(LogLinux, "gcc: looking for toolchain compiler 'gcc%s'...", suffix)

	if x.Gcc, err = utils.UFS.Which("gcc" + suffix); err != nil {
		return err
	}
	if x.Gpp, err = utils.UFS.Which("g++" + suffix); err != nil {
		return err
	}
	if x.Ar, err = utils.UFS.Which("gcc-ar" + suffix); err != nil {
		return err
	}
	if x.InstallDir, err = x.Gpp.Dirname.Realpath(); err != nil {
		return err
	}

	if outp, err := exec.Command(x.Gcc.String(), "--version").Output(); err == nil {
		version := base.UnsafeStringFromBytes(outp)
		if m := re_gccMatchVersion.FindStringSubmatch(version); len(m) == 2 {
			parsed := m[1]
			if n := strings.IndexByte(parsed, '.'); n != -1 {
				parsed = parsed[:n]
			}
			if err = x.ActualVer.Set(parsed); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("can't match gcc version string: %q", version)
		}
	} else {
		errMsg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			errMsg = base.UnsafeStringFromBytes(ee.Stderr)
		}
		base.LogError(LogLinux, "gcc: failed to find g++ version: %s", errMsg)
		return err
	}

	return nil
}
func (x *GccProductInstall) Build(bc utils.BuildContext) (err error) {
	switch x.WantedVer {
	case GCC_LATEST:
		for _, actualVer := range GetLlvmVersions() {
			if err = x.findToolchain("-" + actualVer.String()); err == nil {
				break
			}
		}
	case gcc_any:
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
	if err = bc.NeedFiles(x.Gcc, x.Gpp, x.Ar); err != nil {
		return err
	}
	return nil
}

func (x *GccCompiler) Build(bc utils.BuildContext) error {
	linuxFlags, err := GetLinuxFlags(bc)
	if err != nil {
		return err
	}

	x.ProductInstall, err = GetGccProductInstall(GccProductVer{
		Arch:   x.Arch,
		GccVer: linuxFlags.GccVer,
	}).Need(bc)
	if err != nil {
		return err
	}

	_, err = bc.NeedBuildable(x.ProductInstall)
	if err != nil {
		return err
	}

	x.Version = x.ProductInstall.ActualVer
	x.CompilerRules.Features = base.NewEnumSet(
		compile.COMPILER_ALLOW_CACHING,
		compile.COMPILER_ALLOW_DISTRIBUTION,
		compile.COMPILER_ALLOW_SOURCEMAPPING)

	x.CompilerRules.Executable = x.ProductInstall.Gpp
	x.CompilerRules.Librarian = x.ProductInstall.Ar
	x.CompilerRules.Linker = x.ProductInstall.Gpp

	x.CompilerRules.Environment = internal_io.NewProcessEnvironment()
	x.CompilerRules.Facet = compile.NewFacet()
	facet := &x.CompilerRules.Facet

	facet.Defines.Append(
		"CPP_GCC",
		"CPP_COMPILER=Gcc",
	)

	facet.CompilerOptions.Append(
		"-c", // compile only
	)
	facet.PrecompiledHeaderOptions.Append(
		"-x", "c++-header", "-c", // generate precompiled header
	)

	facet.AddCompilationFlag_NoAnalysis(
		"%1", "-o", "%2", // input file injection"
	)

	facet.AddCompilationFlag(
		"-Wformat=2", "-Wformat-security", // detect Potential Formatting Attack
		"-fsized-deallocation", // https://isocpp.org/files/papers/n3778.html
		"-mlzcnt", "-mpopcnt", "-mbmi",
	)

	if compileFlags, err := compile.GetCompileFlags(bc); err == nil && compileFlags.Benchmark.Get() {
		// https://gcc.gnu.org/onlinedocs/gcc/Developer-Options.html#index-ftime-report
		facet.CompilerOptions.Append("-ftime-report")
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

type GccProductVer struct {
	Arch   compile.ArchType
	GccVer GccVersion
}

func GetGccProductInstall(productVer GccProductVer) utils.BuildFactoryTyped[*GccProductInstall] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (GccProductInstall, error) {
		return GccProductInstall{
			Arch:      productVer.Arch.String(),
			WantedVer: productVer.GccVer,
		}, nil
	})
}

func GetGccCompiler(arch compile.ArchType) utils.BuildFactoryTyped[*GccCompiler] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (GccCompiler, error) {
		gcc := GccCompiler{
			Arch:          arch,
			CompilerRules: compile.NewCompilerRules(compile.NewCompilerAlias("gcc", "g++", arch.String())),
		}
		base.Assert(func() bool {
			var compiler compile.Compiler = &gcc
			return compiler == &gcc
		})
		return gcc, nil
	})
}
