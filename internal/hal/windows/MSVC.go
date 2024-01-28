//go:build windows

package windows

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"

	"github.com/poppolopoppo/ppb/action"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	"github.com/goccy/go-json"
)

// #TODO: expose this as a user option, since pain can go away with an automatic upload to a symstore
// #TODO: support for automatic upload to a symstore? :p
const MSVC_ENABLE_PATHMAP = false

/***************************************
 * MSVC Compiler
 ***************************************/

type MsvcCompiler struct {
	Arch ArchType

	MSC_VER         MsvcVersion
	MinorVer        string
	Host            string
	Target          string
	PlatformToolset string
	VSInstallName   string
	VSInstallPath   Directory
	VCToolsPath     Directory

	CompilerRules

	WindowsFlags WindowsFlags

	MsvcProductInstall      BuildAlias
	ResourceCompilerInstall BuildAlias
	WindowsSDKInstall       BuildAlias
}

func (msvc *MsvcCompiler) GetCompiler() *CompilerRules { return &msvc.CompilerRules }

func (msvc *MsvcCompiler) Serialize(ar base.Archive) {
	ar.Serializable(&msvc.Arch)

	ar.Serializable(&msvc.MSC_VER)
	ar.String(&msvc.MinorVer)
	ar.String(&msvc.Host)
	ar.String(&msvc.Target)
	ar.String(&msvc.PlatformToolset)
	ar.String(&msvc.VSInstallName)
	ar.Serializable(&msvc.VSInstallPath)
	ar.Serializable(&msvc.VCToolsPath)

	ar.Serializable(&msvc.CompilerRules)

	SerializeParsableFlags(ar, &msvc.WindowsFlags)

	ar.Serializable(&msvc.MsvcProductInstall)
	ar.Serializable(&msvc.ResourceCompilerInstall)
	ar.Serializable(&msvc.WindowsSDKInstall)
}

func (msvc *MsvcCompiler) GetWindowsSDK() (*WindowsSDKInstall, error) {
	return FindGlobalBuildable[*WindowsSDKInstall](msvc.WindowsSDKInstall)
}
func (msvc *MsvcCompiler) GetMsvcProduct() (*MsvcProductInstall, error) {
	return FindGlobalBuildable[*MsvcProductInstall](msvc.MsvcProductInstall)
}
func (msvc *MsvcCompiler) GetResourceCompiler() (*ResourceCompiler, error) {
	return FindGlobalBuildable[*ResourceCompiler](msvc.ResourceCompilerInstall)
}

/***************************************
 * Compiler interface
 ***************************************/

func (msvc *MsvcCompiler) Extname(x PayloadType) string {
	switch x {
	case PAYLOAD_EXECUTABLE:
		return ".exe"
	case PAYLOAD_OBJECTLIST:
		return ".obj"
	case PAYLOAD_STATICLIB:
		return ".lib"
	case PAYLOAD_SHAREDLIB:
		return ".dll"
	case PAYLOAD_HEADERUNIT:
		return ".h.ifc"
	case PAYLOAD_PRECOMPILEDHEADER:
		return ".pch"
	case PAYLOAD_HEADERS:
		return ".h"
	case PAYLOAD_SOURCES:
		return ".cpp"
	case PAYLOAD_DEBUGSYMBOLS:
		return ".pdb"
	case PAYLOAD_DEPENDENCIES:
		return ".obj.json"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (msvc *MsvcCompiler) CppRtti(f *Facet, enabled bool) {
	if enabled {
		f.AddCompilationFlag("/GR")
	} else {
		f.AddCompilationFlag("/GR-")
	}
}
func (msvc *MsvcCompiler) CppStd(f *Facet, std CppStdType) {
	maxSupported := getCppStdFromMsc(msvc.MSC_VER)
	if int32(std) > int32(maxSupported) {
		std = maxSupported
	}
	switch std {
	case CPPSTD_LATEST, CPPSTD_20:
		f.AddCompilationFlag("/std:c++20")
	case CPPSTD_17:
		f.AddCompilationFlag("/std:c++17")
	case CPPSTD_14:
		f.AddCompilationFlag("/std:c++14")
	case CPPSTD_11:
		f.AddCompilationFlag("/std:c++11")
	default:
		base.UnexpectedValue(std)
	}

}
func (msvc *MsvcCompiler) AllowCaching(u *Unit, payload PayloadType) (result action.CacheModeType) {
	switch payload {
	case PAYLOAD_PRECOMPILEDHEADER:
		result = action.CACHE_NONE
		base.LogVeryVerbose(LogWindows, "%v/%v: can't cache precompiled headers (can still cache objects compiled with PCH)", u, payload)
	case PAYLOAD_OBJECTLIST:
		if u.DebugSymbols == DEBUG_EMBEDDED {
			result = action.CACHE_READWRITE
		} else {
			result = action.CACHE_NONE
			base.LogVeryVerbose(LogWindows, "%v/%v: can't use caching with %v debug symbols", u, payload, u.DebugSymbols)
		}
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		if u.Incremental.Get() {
			result = action.CACHE_NONE
			base.LogVeryVerbose(LogWindows, "%v/%v: can't use caching with incremental linker", u, payload)
		} else if u.DebugFastLink.Get() {
			result = action.CACHE_NONE
			base.LogVeryVerbose(LogWindows, "%v/%v: can't use caching with debug fast link", u, payload)
		} else {
			result = action.CACHE_READWRITE
		}
	case PAYLOAD_HEADERUNIT, PAYLOAD_STATICLIB, PAYLOAD_DEBUGSYMBOLS:
		result = action.CACHE_READWRITE
	}
	if result == action.CACHE_INHERIT {
		result = action.CACHE_NONE
	}
	if result != action.CACHE_NONE && !u.Deterministic.Get() {
		result = action.CACHE_NONE
		base.LogVeryVerbose(LogWindows, "%v/%v: can't use caching without determinism", u, payload)
	}
	return result
}
func (msvc *MsvcCompiler) AllowDistribution(u *Unit, payload PayloadType) (result action.DistModeType) {
	switch payload {
	case PAYLOAD_PRECOMPILEDHEADER:
		result = action.DIST_NONE
	case PAYLOAD_OBJECTLIST:
		if u.DebugSymbols == DEBUG_EMBEDDED {
			result = action.DIST_ENABLE
		} else {
			result = action.DIST_NONE
			base.LogVeryVerbose(LogWindows, "%v/%v: can't use distribution with %v debug symbols", u, payload, u.DebugSymbols)
		}
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB, PAYLOAD_STATICLIB, PAYLOAD_HEADERUNIT, PAYLOAD_DEBUGSYMBOLS:
		result = action.DIST_ENABLE
	}
	if result == action.DIST_INHERIT {
		result = action.DIST_NONE
	}
	return result
}
func (msvc *MsvcCompiler) AllowResponseFile(u *Unit, payload PayloadType) (result CompilerSupportType) {
	switch payload {
	case PAYLOAD_OBJECTLIST, PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER, PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB, PAYLOAD_STATICLIB, PAYLOAD_DEBUGSYMBOLS:
		result = COMPILERSUPPORT_ALLOWED
	}
	if result == COMPILERSUPPORT_INHERIT {
		result = COMPILERSUPPORT_UNSUPPORTED
	}
	return result
}
func (msvc *MsvcCompiler) AllowEditAndContinue(u *Unit, payload PayloadType) (result CompilerSupportType) {
	switch payload {
	case PAYLOAD_OBJECTLIST:
		if u.CompilerOptions.Contains("/ZI") {
			return COMPILERSUPPORT_ALLOWED
		}
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		if u.LinkerOptions.Contains("/EDITANDCONTINUE") {
			return COMPILERSUPPORT_ALLOWED
		}
	}
	return COMPILERSUPPORT_UNSUPPORTED
}
func (msvc *MsvcCompiler) Define(f *Facet, def ...string) {
	for _, x := range def {
		f.AddCompilationFlag(fmt.Sprint("/D", x))
	}
}
func (msvc *MsvcCompiler) DebugSymbols(u *Unit) {
	artifactPDB := u.OutputFile.ReplaceExt(".pdb")

	switch u.DebugSymbols {
	case DEBUG_DISABLED:
		u.LinkerOptions.Append("/DEBUG:NONE")
		return

	case DEBUG_EMBEDDED:
		u.AddCompilationFlag_NoPreprocessor("/Z7")

		if u.Payload.HasLinker() {
			u.SymbolsFile = artifactPDB
			u.LinkerOptions.Append("/DEBUG", "/PDB:"+MakeLocalFilename(artifactPDB))

			if u.DebugFastLink.Get() {
				u.LinkerOptions.Append("/DEBUG:FASTLINK")
			}
		}

	case DEBUG_SYMBOLS:
		u.SymbolsFile = artifactPDB
		if u.Payload.HasLinker() {
			u.SymbolsFile = artifactPDB
			u.LinkerOptions.Append("/DEBUG", "/PDB:"+MakeLocalFilename(artifactPDB))

			if u.DebugFastLink.Get() {
				u.LinkerOptions.Append("/DEBUG:FASTLINK")
			}
		}

		u.AddCompilationFlag_NoPreprocessor("/Zi", "/Zf", "/FS", "/Fd"+MakeLocalFilename(artifactPDB))

	case DEBUG_HOTRELOAD:
		u.SymbolsFile = artifactPDB

		if u.Payload.HasLinker() {
			u.LinkerOptions.Append("/DEBUG", "/EDITANDCONTINUE", "/PDB:"+MakeLocalFilename(artifactPDB))

			if u.Incremental.Get() && !u.LinkerOptions.Contains("/INCREMENTAL:NO") {
				u.LinkerOptions.AppendUniq("/INCREMENTAL")
			}

			if u.DebugFastLink.Get() {
				u.LinkerOptions.Append("/DEBUG:FASTLINK")
			}
		}

		// need to use a dedicated readable PDB for compiled units when using EditAndContinue
		editAndContinuePDB := u.IntermediateDir.File("EditAndContinue.pdb")

		u.AddCompilationFlag_NoPreprocessor(
			"/wd4657", // expression involves a data type that is new since the latest build
			"/wd4656", // data type is new or has changed since the latest build, or is defined differently elsewhere
			"/ZI", "/Zf", "/FS", "/Fd"+MakeLocalFilename(editAndContinuePDB))

	default:
		base.UnexpectedValue(u.DebugSymbols)
	}
}
func (msvc *MsvcCompiler) Link(f *Facet, lnk LinkType) {
	switch lnk {
	case LINK_STATIC:
		return // nothing to do
	case LINK_DYNAMIC:
		// https://msdn.microsoft.com/en-us/library/527z7zfs.aspx
		f.LinkerOptions.Append("/DLL")
	default:
		base.UnexpectedValue(lnk)
	}
}
func (msvc *MsvcCompiler) PrecompiledHeader(u *Unit) {
	switch u.PCH {
	case PCH_MONOLITHIC, PCH_SHARED:
		u.CompilerOptions.Append(
			"/FI"+u.PrecompiledHeader.Basename,
			"/Yu"+u.PrecompiledHeader.Basename,
			"/Fp"+MakeLocalFilename(u.PrecompiledObject))
		if u.PCH != PCH_SHARED {
			u.PrecompiledHeaderOptions.Append("/Yc" + u.PrecompiledHeader.Basename)
		}
	case PCH_HEADERUNIT:
		headerFile := MakeLocalFilename(u.PrecompiledHeader)
		if msvc.WindowsFlags.TranslateInclude.Get() {
			u.CompilerOptions.Append("/translateInclude") // converts #include to #import automatically if an ifc is available for the header
		}
		u.CompilerOptions.Append(
			// https://learn.microsoft.com/en-us/cpp/build/walkthrough-import-stl-header-units?view=msvc-170#approach1
			"/headerUnit", fmt.Sprintf("%v=%v", headerFile, MakeLocalFilename(u.PrecompiledObject)),
			"/reference", MakeLocalFilename(u.PrecompiledObject),
			"/FI"+headerFile)
	case PCH_DISABLED:
	default:
		base.UnexpectedValue(u.PCH)
	}
}
func (msvc *MsvcCompiler) Sanitizer(f *Facet, sanitizer SanitizerType) {
	switch sanitizer {
	case SANITIZER_NONE:
		return
	case SANITIZER_ADDRESS:
		// https://devblogs.microsoft.com/cppblog/addresssanitizer-asan-for-windows-with-msvc/
		f.Defines.Append("USE_PPE_SANITIZER=1")
		f.AddCompilationFlag_NoAnalysis("/fsanitize=address", "/fsanitize-address-use-after-return")
	default:
		base.UnexpectedValue(sanitizer)
	}
}

func (msvc *MsvcCompiler) ForceInclude(f *Facet, inc ...Filename) {
	for _, x := range inc {
		f.AddCompilationFlag_NoAnalysis("/FI" + x.Relative(UFS.Source))
	}
}
func (msvc *MsvcCompiler) IncludePath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("/I" + MakeLocalDirectory(x))
	}
}
func (msvc *MsvcCompiler) ExternIncludePath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		f.AddCompilationFlag_NoAnalysis("/external:I" + MakeLocalDirectory(x))
	}
}
func (msvc *MsvcCompiler) SystemIncludePath(facet *Facet, dirs ...Directory) {
	msvc.ExternIncludePath(facet, dirs...)
}
func (msvc *MsvcCompiler) Library(f *Facet, lib ...Filename) {
	for _, x := range lib {
		libInc := MakeLocalFilename(x)
		f.LibrarianOptions.Append(libInc)
		f.LinkerOptions.Append(libInc)
	}
}
func (msvc *MsvcCompiler) LibraryPath(f *Facet, dirs ...Directory) {
	for _, x := range dirs {
		libPath := "/LIBPATH:" + MakeLocalDirectory(x)
		f.LibrarianOptions.Append(libPath)
		f.LinkerOptions.Append(libPath)
	}
}
func (msvc *MsvcCompiler) GetPayloadOutput(u *Unit, payload PayloadType, file Filename) Filename {
	if payload == PAYLOAD_OBJECTLIST && u.DebugSymbols == DEBUG_HOTRELOAD {
		// cl.exe creates a new file with all letters switched to lowercase when recompiling a TU for hot-reload, which defeats all the efforts made for conserving file case everywhere
		// #TODO: find a better workaround, for now we will generate lower case TU when using hot-reload (should report the issue first)

		workaround := Filename{
			Dirname:  file.Dirname,
			Basename: strings.ToLower(file.Basename),
		}
		base.LogTrace(action.LogAction, "force lower case for output because MSVC hotreload:\n\torig: %q\n\thack: %q", file, workaround)

		file = workaround
	}
	return file.ReplaceExt(msvc.Extname(payload))
}
func (msvc *MsvcCompiler) CreateAction(u *Unit, payload PayloadType, model *action.ActionModel) action.Action {
	if internal_io.OnRunCommandWithDetours != nil || // use IO detouring with DLL injection
		!model.Options.Has(action.OPT_ALLOW_SOURCECONTROL) { // rely on internal logic to track dependencies
		rules := model.CreateActionRules()
		return &rules

	} else {
		// use explicit compiler support with /sourceDependencies
		return NewMsvcSourceDependenciesAction(model,
			model.ExportFile.ReplaceExt(msvc.Extname(PAYLOAD_DEPENDENCIES)))
	}
}

func (msvc *MsvcCompiler) AddResources(compileEnv *CompileEnv, u *Unit, rc Filename) error {
	base.LogVeryVerbose(LogWindows, "MSVC: add resource compiler custom unit to %v", u.Alias())

	resourceCompiler, err := msvc.GetResourceCompiler()
	if err != nil {
		return err
	}

	resources := CustomUnit{
		Unit: Unit{
			TargetAlias:     u.TargetAlias,
			ModuleDir:       u.ModuleDir,
			GeneratedDir:    u.GeneratedDir,
			IntermediateDir: u.IntermediateDir,
			Payload:         PAYLOAD_OBJECTLIST,
			Facet:           u.Facet,
			Source: ModuleSource{
				SourceFiles: FileSet{rc},
			},
			CompilerAlias: resourceCompiler.GetCompiler().CompilerAlias,
			CppRules: CppRules{
				PCH:   PCH_DISABLED,
				Unity: UNITY_DISABLED,
			},
		},
	}

	resources.TargetAlias.ModuleName += "-RC"
	resources.AnalysisOptions.Clear()
	resources.CompilerOptions.Clear()
	resources.PreprocessorOptions.Clear()
	resources.LibrarianOptions.Clear()
	resources.LinkerOptions.Clear()

	if err := resources.Decorate(compileEnv, resources.GetCompiler()); err != nil {
		return err
	}
	resources.Append(resources.GetCompiler()) // compiler options need to be at the end of command-line

	u.CustomUnits.Append(resources)

	return nil
}

func (msvc *MsvcCompiler) GetPathmap() string {
	if MSVC_ENABLE_PATHMAP {
		// #TODO: debugging with this is painful...
		return fmt.Sprintf("/pathmap:%v=.", UFS.Root)
	} else {
		return fmt.Sprintf("/pathmap:%v=%v", UFS.Root, UFS.Root)
	}
}
func (msvc *MsvcCompiler) Decorate(compileEnv *CompileEnv, u *Unit) error {
	if u.LinkerVerbose.Get() {
		u.LinkerOptions.Append(
			"/VERBOSE",
			"/VERBOSE:LIB",
			"/VERBOSE:ICF",
			"/VERBOSE:REF",
			"/VERBOSE:INCR",
			"/VERBOSE:UNUSEDLIBS",
		)
	}

	// set architecture options
	switch compileEnv.GetPlatform().Arch {
	case ARCH_X86:
		u.LibrarianOptions.Append("/MACHINE:x86")
		u.LinkerOptions.Append("/MACHINE:x86")
		u.LibraryPaths.Append(
			msvc.VSInstallPath.Folder("VC", "Tools", "MSVC", msvc.MinorVer, "lib", "x86"),
			msvc.VSInstallPath.Folder("VC", "Auxiliary", "VS", "lib", "x86"))

		if u.DebugSymbols != DEBUG_HOTRELOAD {
			u.LinkerOptions.Append("/SAFESEH")
		}

	case ARCH_X64:
		u.LibrarianOptions.Append("/MACHINE:x64")
		u.LinkerOptions.Append("/MACHINE:x64")
		u.LibraryPaths.Append(
			msvc.VSInstallPath.Folder("VC", "Tools", "MSVC", msvc.MinorVer, "lib", "x64"),
			msvc.VSInstallPath.Folder("VC", "Auxiliary", "VS", "lib", "x64"))
	default:
		base.UnexpectedValue(compileEnv.GetPlatform().Arch)
	}

	// sanitizer sanity check
	if u.Sanitizer.IsEnabled() && u.Sanitizer != SANITIZER_ADDRESS {
		base.LogWarning(LogWindows, "%v: sanitizer %v is not supported on windows", u, u.Sanitizer)
		u.Sanitizer = SANITIZER_NONE
	}

	// hot-reload can override LTCG
	if u.DebugSymbols == DEBUG_HOTRELOAD {
		if u.LTO.Get() {
			base.LogWarning(LogWindows, "%v: can't enable LTO while HOTRELOAD is enabled", u)
			u.LTO.Disable()
		}
		if u.LinkerOptions.Contains("/LTCG") || u.LinkerOptions.Contains("/LTCG:INCREMENTAL") {
			base.LogWarning(LogWindows, "%v: LTCG found while HOTRELOAD is enabled, reverting to SYMBOLS", u)
			u.DebugSymbols = DEBUG_SYMBOLS
		}
	}

	// set compiler options from configuration
	switch compileEnv.GetConfig().ConfigType {
	case CONFIG_DEBUG:
		decorateMsvcConfig_Debug(msvc, u)
	case CONFIG_FASTDEBUG:
		decorateMsvcConfig_FastDebug(msvc, u)
	case CONFIG_DEVEL:
		decorateMsvcConfig_Devel(msvc, u)
	case CONFIG_TEST:
		decorateMsvcConfig_Test(msvc, u)
	case CONFIG_SHIPPING:
		decorateMsvcConfig_Shipping(msvc, u)
	default:
		base.UnexpectedValue(compileEnv.GetConfig().ConfigType)
	}

	// check if C++20 at least is enabled
	if u.CppStd >= CPPSTD_20 {
		// C++20 deprecates /Gm
		// Command line warning D9035 : option 'Gm' has been deprecated and will be removed in a future release
		// Command line error D8016 : '/Gm' and '/std:c++20' command-line options are incompatible
		u.RemoveCompilationFlag("/Gm-", "/Gm")

		// Use C++20 header units instead of precompiled headers
		if msvc.WindowsFlags.TranslateInclude.Get() && (u.PCH == PCH_SHARED || u.PCH == PCH_MONOLITHIC) {
			u.PCH = PCH_HEADERUNIT
			base.LogVeryVerbose(LogWindows, "%v: generate a C++20 header unit instead of a precompiled header with /translateInclude", u)
		}
	}

	// check AVX2 support
	if u.Avx2.Get() {
		u.AddCompilationFlag("/arch:AVX2")
	}

	// set default thread stack size
	stackSize := msvc.WindowsFlags.StackSize
	if u.Sanitizer.IsEnabled() {
		stackSize *= 2
		base.LogVeryVerbose(LogWindows, "%v: doubling thread stack size due to msvc sanitizer (%d)", u, stackSize)
	}
	u.AddCompilationFlag(fmt.Sprintf("/F%d", stackSize))
	u.LinkerOptions.Append(fmt.Sprintf("/STACK:%d", stackSize))

	if msvc.WindowsFlags.Analyze.Get() {
		base.LogVeryVerbose(LogWindows, "%v: using msvc static analysis", u)

		msvcProduct, err := msvc.GetMsvcProduct()
		if err != nil {
			return err
		}

		u.AddCompilationFlag_NoAnalysis(
			"/analyze",
			"/analyze:external-", // disable analysis of external headers
			fmt.Sprint("/analyse:stacksize", stackSize),
			fmt.Sprintf("/analyze:plugin\"%v\"", msvcProduct.VcToolsHostPath().File("EspXEngine.dll")),
		)
		u.Defines.Append("ANALYZE")
	}

	// set dependent linker options

	if u.Sanitizer.IsEnabled() {
		base.LogVeryVerbose(LogWindows, "%v: using sanitizer %v", u, u.Sanitizer)
		// https://github.com/google/sanitizers/wiki/AddressSanitizerFlags
		asanOptions := "check_initialization_order=1:detect_stack_use_after_return=1:windows_hook_rtl_allocators=1"
		// - use_sigaltstack=0 to workaround this issue: https://github.com/google/sanitizers/issues/1171
		// asanOptions += ":use_sigaltstack=0"
		// - detect_leaks=1 is not supported on Windows (visual studio 17.7.2)
		// asanOptions += ":detect_leaks=1"
		if compileEnv.Tags.Has(TAG_DEBUG) {
			asanOptions += ":debug=1:verbose=1"
		}
		u.Environment.Append("ASAN_OPTIONS", asanOptions)

		if u.Incremental.Get() {
			base.LogWarning(LogWindows, "%v: can't enable incremental linker while %v is enabled", u, u.Sanitizer)
			u.Incremental.Assign(false)
		}

		if u.CompilerOptions.RemoveAll("/INCREMENTAL") {
			base.LogVeryVerbose(LogWindows, "%v: remove /INCREMENTAL due to %v", u, u.Sanitizer)
		}
		if u.CompilerOptions.RemoveAll("/LTCG") {
			base.LogVeryVerbose(LogWindows, "%v: remove /LTCG due to %v", u, u.Sanitizer)
		}
		if u.CompilerOptions.RemoveAll("/LTCG:INCREMENTAL") {
			base.LogVeryVerbose(LogWindows, "%v: remove /LTCG:INCREMENTAL due to %v", u, u.Sanitizer)
		}
	}

	if u.Deterministic.Get() {
		switch u.DebugSymbols {
		case DEBUG_SYMBOLS, DEBUG_EMBEDDED, DEBUG_DISABLED:
			// https://nikhilism.com/post/2020/windows-deterministic-builds/
			u.Incremental.Disable()
			pathMap := msvc.GetPathmap()
			u.AddCompilationFlag("/Brepro", "/experimental:deterministic", pathMap, "/d1nodatetime")
			//u.AddCompilationFlag("/d1trimfile:"+UFS.Root.String()) // implied by /experimental:deterministic + /pathmap:
			u.PrecompiledHeaderOptions.Append("/wd5049") // Embedding a full path may result in machine-dependent output (always happen with PCH)
			u.LibrarianOptions.Append("/Brepro", "/experimental:deterministic")
			if !u.Incremental.Get() {
				u.LinkerOptions.Append("/Brepro", "/experimental:deterministic", pathMap, "/pdbaltpath:%_PDB%")
			}
		case DEBUG_HOTRELOAD:
			base.LogWarning(LogWindows, "%v: can't enable determinism while %v is enabled", u, u.DebugSymbols)
		default:
			base.UnexpectedValuePanic(u.DebugSymbols, u.DebugSymbols)
		}
	}

	if u.Incremental.Get() {
		base.LogVeryVerbose(LogWindows, "%v: using msvc incremental linker", u)
		if u.LinkerOptions.Contains("/INCREMENTAL") {
			u.LinkerOptions.Remove("/LTCG")
		} else if u.LinkerOptions.Contains("/LTCG") {
			u.LinkerOptions.Remove("/LTCG")
			u.LinkerOptions.Append("/LTCG:INCREMENTAL")
			u.LinkerOptions.Remove("/OPT:NOREF")
		} else if !u.LinkerOptions.Contains("/LTCG:INCREMENTAL") {
			u.LinkerOptions.Append("/INCREMENTAL")
		} else {
			u.LinkerOptions.Remove("/OPT:NOREF")
		}
	} else if !u.LinkerOptions.Contains("/INCREMENTAL") {
		base.LogVeryVerbose(LogWindows, "%v: using non-incremental msvc linker", u)
		u.LinkerOptions.Append("/INCREMENTAL:NO")
	}

	// eventually detects resource file to compile on Windows
	if u.Payload == PAYLOAD_EXECUTABLE || u.Payload == PAYLOAD_SHAREDLIB {
		resource_rc := u.ModuleDir.File("resource.rc")
		if resource_rc.Exists() {
			if err := msvc.AddResources(compileEnv, u, resource_rc); err != nil {
				return err
			}
		}
	}

	// enable perfSDK if necessary
	if msvc.WindowsFlags.PerfSDK.Get() {
		base.LogVeryVerbose(LogWindows, "%v: using Windows PerfSDK", u)
		var perfSDK Directory
		switch compileEnv.GetPlatform().Arch {
		case ARCH_X64:
			perfSDK = msvc.VSInstallPath.Folder("Team Tools", "Performance Tools", "x64", "PerfSDK")
		case ARCH_X86:
			perfSDK = msvc.VSInstallPath.Folder("Team Tools", "Performance Tools", "PerfSDK")
		default:
			base.UnexpectedValue(compileEnv.GetPlatform().Arch)
		}
		u.Defines.Append("WITH_VISUALSTUDIO_PERFSDK")
		u.ExternIncludePaths.Append(perfSDK)
		u.LibraryPaths.Append(perfSDK)
	}

	// register extra files generated by the compiler
	switch u.Payload {
	case PAYLOAD_EXECUTABLE, PAYLOAD_SHAREDLIB:
		if u.Payload == PAYLOAD_SHAREDLIB {
			u.ExtraFiles.Append(u.OutputFile.ReplaceExt(".lib"))
			u.ExtraFiles.Append(u.OutputFile.ReplaceExt(".exp"))
		}
		if u.LinkerOptions.Contains("/INCREMENTAL") {
			u.ExtraFiles.Append(u.OutputFile.ReplaceExt(".ilk"))
		}

	case PAYLOAD_STATICLIB:
	case PAYLOAD_HEADERUNIT, PAYLOAD_PRECOMPILEDHEADER:
	case PAYLOAD_OBJECTLIST, PAYLOAD_HEADERS:
	default:
		base.UnexpectedValuePanic(u.Payload, u.Payload)
	}

	return nil
}

/***************************************
 * Compiler options per configuration
 ***************************************/

func msvc_CXX_runtimeLibrary(u *Unit, staticCrt bool, debug bool) {
	if u.CompilerOptions.Any("/MD", "/MDd", "/MT", "/MTd") {
		// don't override user configuration
		return
	}

	var suffix string
	if debug {
		suffix = "d"
		u.Defines.AppendUniq("_DEBUG")
		u.Defines.Remove("NDEBUG")
	} else {
		u.Defines.AppendUniq("NDEBUG")
		u.Defines.Remove("_DEBUG")
	}

	var runtimeFlag string
	if staticCrt {
		runtimeFlag = "/MT"
		base.LogVeryVerbose(LogWindows, "%v: using msvc static CRT libraries %s%s (debug=%v)", u, runtimeFlag, suffix, debug)
		u.AddCompilationFlag(
			"LIBCMT"+suffix+".lib",
			"libvcruntime"+suffix+".lib",
			"libucrt"+suffix+".lib")
	} else {
		base.LogVeryVerbose(LogWindows, "%v: using msvc dynamic CRT libraries %s%s (debug=%v)", u, runtimeFlag, suffix, debug)
		runtimeFlag = "/MD"
		u.Defines.Append("_DLL")
	}

	u.AddCompilationFlag(runtimeFlag + suffix)
}
func msvc_CXX_linkTimeCodeGeneration(u *Unit, enabled bool) {
	if !u.LinkerOptions.Any("/LTCG", "/LTCG:OFF", "/LTCG:INCREMENTAL") {
		if enabled {
			base.LogVeryVerbose(LogWindows, "%v: using msvc link time code generation", u)
			u.LinkerOptions.Append("/LTCG")
		} else {
			base.LogVeryVerbose(LogWindows, "%v: disabling msvc link time code generation", u)
			u.LinkerOptions.Append("/LTCG:OFF")
		}
	}
}
func msvc_CXX_runtimeChecks(u *Unit, enabled bool, rtc1 bool) {
	if enabled {
		base.LogVeryVerbose(LogWindows, "%v: using msvc runtime checks and control flow guard", u)
		// https://msdn.microsoft.com/fr-fr/library/jj161081(v=vs.140).aspx
		u.AddCompilationFlag("/GS", "/sdl")
		if rtc1 {
			base.LogVeryVerbose(LogWindows, "%v: using msvc RTC1 checks", u)
			// https://msdn.microsoft.com/fr-fr/library/8wtf2dfz.aspx
			u.AddCompilationFlag("/RTC1")
		}
		u.LinkerOptions.Append("/GUARD:CF")
	} else {
		base.LogVeryVerbose(LogWindows, "%v: disabling msvc runtime checks", u)
		u.AddCompilationFlag("/GS-", "/sdl-")
		u.LinkerOptions.Append("/GUARD:NO")
	}
}
func msvc_STL_debugHeap(u *Unit, enabled bool) {
	if !enabled {
		base.LogVeryVerbose(LogWindows, "%v: disabling msvc debug heap", u)
		u.Defines.Append("_NO_DEBUG_HEAP=1")
	}
}
func msvc_STL_iteratorDebug(u *Unit, enabled bool) {
	if enabled && u.CompilerOptions.Any("/MDd", "/MTd") {
		base.LogVeryVerbose(LogWindows, "%v: enable msvc STL iterator debugging", u)
		u.Defines.Append(
			"_SECURE_SCL=1",             // https://msdn.microsoft.com/fr-fr/library/aa985896.aspx
			"_ITERATOR_DEBUG_LEVEL=2",   // https://msdn.microsoft.com/fr-fr/library/hh697468.aspx
			"_HAS_ITERATOR_DEBUGGING=1") // https://msdn.microsoft.com/fr-fr/library/aa985939.aspx")
	} else {
		base.LogVeryVerbose(LogWindows, "%v: disable msvc STL iterator debugging", u)
		u.Defines.Append(
			"_SECURE_SCL=0",             // https://msdn.microsoft.com/fr-fr/library/aa985896.aspx
			"_ITERATOR_DEBUG_LEVEL=0",   // https://msdn.microsoft.com/fr-fr/library/hh697468.aspx
			"_HAS_ITERATOR_DEBUGGING=0") // https://msdn.microsoft.com/fr-fr/library/aa985939.aspx
	}
}

func decorateMsvcConfig_Debug(msvc *MsvcCompiler, u *Unit) {
	useSanitizer := u.Sanitizer.IsEnabled()
	u.AddCompilationFlag("/Od", "/Oy-", "/Gm-", "/Gw-")
	u.LinkerOptions.Append("/DYNAMICBASE:NO", "/HIGHENTROPYVA:NO", "/OPT:NOREF", "/OPT:NOICF")
	msvc_CXX_runtimeLibrary(u, msvc.WindowsFlags.StaticCRT.Get(), !useSanitizer)
	msvc_CXX_linkTimeCodeGeneration(u, u.LTO.Get())
	msvc_CXX_runtimeChecks(u, u.RuntimeChecks.Get() && !useSanitizer, !useSanitizer)
	msvc_STL_debugHeap(u, !useSanitizer)
	msvc_STL_iteratorDebug(u, !useSanitizer)
}
func decorateMsvcConfig_FastDebug(msvc *MsvcCompiler, u *Unit) {
	useSanitizer := u.Sanitizer.IsEnabled()
	u.AddCompilationFlag("/Od", "/Ob1", "/Oy-", "/Gw-", "/Gm")
	u.LinkerOptions.Append("/DYNAMICBASE:NO", "/HIGHENTROPYVA:NO")
	msvc_CXX_runtimeLibrary(u, msvc.WindowsFlags.StaticCRT.Get(), !useSanitizer)
	msvc_CXX_linkTimeCodeGeneration(u, u.LTO.Get())
	msvc_CXX_runtimeChecks(u, false, false)
	msvc_STL_debugHeap(u, !useSanitizer)
	msvc_STL_iteratorDebug(u, !useSanitizer)
}
func decorateMsvcConfig_Devel(msvc *MsvcCompiler, u *Unit) {
	useSanitizer := u.Sanitizer.IsEnabled()
	u.AddCompilationFlag("/O2", "/Oy-", "/GA", "/Gm-", "/Zo", "/GL")
	u.LinkerOptions.Append("/DYNAMICBASE:NO", "/HIGHENTROPYVA:NO", "/OPT:NOICF")
	msvc_CXX_runtimeLibrary(u, msvc.WindowsFlags.StaticCRT.Get(), false)
	msvc_CXX_linkTimeCodeGeneration(u, u.LTO.Get())
	msvc_CXX_runtimeChecks(u, u.RuntimeChecks.Get() && !useSanitizer, false)
	msvc_STL_debugHeap(u, false)
	msvc_STL_iteratorDebug(u, false)
}
func decorateMsvcConfig_Test(msvc *MsvcCompiler, u *Unit) {
	u.AddCompilationFlag("/O2", "/Ob3", "/Gw", "/Gm-", "/Gy", "/GL", "/GA", "/Zo")
	u.LinkerOptions.Append("/DYNAMICBASE", "/HIGHENTROPYVA", "/PROFILE", "/OPT:REF")
	msvc_CXX_runtimeLibrary(u, msvc.WindowsFlags.StaticCRT.Get(), false)
	msvc_CXX_linkTimeCodeGeneration(u, u.LTO.Get())
	msvc_CXX_runtimeChecks(u, false, false)
	msvc_STL_debugHeap(u, false)
	msvc_STL_iteratorDebug(u, false)
}
func decorateMsvcConfig_Shipping(msvc *MsvcCompiler, u *Unit) {
	u.AddCompilationFlag("/O2", "/Ob3", "/Gw", "/Gm-", "/Gy", "/GL", "/GA", "/Zo-")
	u.LinkerOptions.Append("/DYNAMICBASE", "/HIGHENTROPYVA", "/OPT:REF", "/OPT:ICF=3")
	msvc_CXX_runtimeLibrary(u, msvc.WindowsFlags.StaticCRT.Get(), false)
	msvc_CXX_linkTimeCodeGeneration(u, u.LTO.Get())
	msvc_CXX_runtimeChecks(u, false, false)
	msvc_STL_debugHeap(u, false)
	msvc_STL_iteratorDebug(u, false)
}

/***************************************
 * Compiler detection
 ***************************************/

var MSVC_VSWHERE_EXE = UFS.Internal.Folder("hal", "windows", "bin").File("vswhere.exe")

type VsWhereCatalog struct {
	ProductDisplayVersion string
	ProductLineVersion    string
}

func (x *VsWhereCatalog) Serialize(ar base.Archive) {
	ar.String(&x.ProductDisplayVersion)
	ar.String(&x.ProductLineVersion)
}

type VsWhereEntry struct {
	InstallationName string
	InstallationPath string
	Catalog          VsWhereCatalog
}

func (x *VsWhereEntry) Serialize(ar base.Archive) {
	ar.String(&x.InstallationName)
	ar.String(&x.InstallationPath)
	ar.Serializable(&x.Catalog)
}

type MsvcProductInstall struct {
	Arch      string
	WantedVer MsvcVersion
	Insider   bool

	ActualVer      MsvcVersion
	HostArch       string
	Selected       VsWhereEntry
	VsInstallPath  Directory
	VcToolsPath    Directory
	VcToolsFileSet FileSet

	Cl_exe   Filename
	Lib_exe  Filename
	Link_exe Filename
}

func (x *MsvcProductInstall) Commond7IdePath() Directory {
	return x.VsInstallPath.Folder("Common7", "IDE")
}
func (x *MsvcProductInstall) VcToolsHostPath() Directory {
	return x.VcToolsPath.Folder("bin", "Host"+x.HostArch, x.Arch)
}

func (x *MsvcProductInstall) Alias() BuildAlias {
	variant := "Stable"
	if x.Insider {
		variant = "Insider"
	}
	return MakeBuildAlias("HAL", "Windows", "MSVC", x.WantedVer.String(), x.Arch, variant)
}
func (x *MsvcProductInstall) Serialize(ar base.Archive) {
	ar.String(&x.Arch)
	ar.Serializable(&x.WantedVer)
	ar.Bool(&x.Insider)

	ar.Serializable(&x.ActualVer)
	ar.String(&x.HostArch)
	ar.Serializable(&x.Selected)
	ar.Serializable(&x.VsInstallPath)
	ar.Serializable(&x.VcToolsPath)
	ar.Serializable(&x.VcToolsFileSet)

	ar.Serializable(&x.Cl_exe)
	ar.Serializable(&x.Lib_exe)
	ar.Serializable(&x.Link_exe)
}
func (x *MsvcProductInstall) Build(bc BuildContext) error {
	x.HostArch = getWindowsHostPlatform()

	name := fmt.Sprintf("MSVC_Host%v_%v", x.HostArch, x.Arch)
	if x.Insider {
		name += "_Insider"
	}

	// https://github.com/microsoft/vswhere/wiki/Find-VC#powershell
	var args = []string{
		"-format", "json",
		"-latest",
		"-products", "*",
		"-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
	}

	switch x.WantedVer {
	case msc_ver_any: // don't filter
	case MSC_VER_2022:
		args = append(args, "-version", "[17.0,18.0)")
	case MSC_VER_2019:
		args = append(args, "-version", "[16.0,17.0)")
	case MSC_VER_2017:
		args = append(args, "-version", "[15.0,16.0)")
	case MSC_VER_2015:
		args = append(args, "-version", "[14.0,15.0)")
	case MSC_VER_2013:
		args = append(args, "-version", "[13.0,14.0)")
	default:
		base.UnexpectedValue(x.WantedVer)
	}

	if x.Insider {
		args = append(args, "-prerelease")
	}

	cmd := exec.Command(MSVC_VSWHERE_EXE.String(), args...)

	var entries []VsWhereEntry
	if outp, err := cmd.Output(); err != nil {
		return err
	} else if len(outp) > 0 {
		if err := json.Unmarshal(outp, &entries); err != nil {
			return err
		}
	}

	if len(entries) == 0 {
		return fmt.Errorf("msvc: vswhere did not find any compiler")
	}

	x.Selected = entries[0]
	x.VsInstallPath = MakeDirectory(x.Selected.InstallationPath)
	if _, err := x.VsInstallPath.Info(); err != nil {
		return err
	}

	if err := x.ActualVer.Set(x.Selected.Catalog.ProductLineVersion); err != nil {
		return err
	}

	var vcToolsVersion string
	vcToolsVersionFile := x.VsInstallPath.Folder("VC", "Auxiliary", "Build").File("Microsoft.VCToolsVersion.default.txt")

	if data, err := os.ReadFile(vcToolsVersionFile.String()); err == nil {
		vcToolsVersion = strings.TrimSpace(base.UnsafeStringFromBytes(data))
	} else {
		return err
	}

	x.VcToolsPath = x.VsInstallPath.Folder("VC", "Tools", "MSVC", vcToolsVersion)

	vcToolsHostPath := x.VcToolsHostPath()

	x.VcToolsFileSet = FileSet{}
	x.VcToolsFileSet.Append(
		vcToolsHostPath.File("c1.dll"),
		vcToolsHostPath.File("c1xx.dll"),
		vcToolsHostPath.File("c2.dll"),
		vcToolsHostPath.File("msobj140.dll"),
		vcToolsHostPath.File("mspdb140.dll"),
		vcToolsHostPath.File("mspdbcore.dll"),
		vcToolsHostPath.File("mspdbsrv.exe"),
		vcToolsHostPath.File("mspft140.dll"),
		vcToolsHostPath.File("msvcp140.dll"),
		vcToolsHostPath.File("msvcp140_atomic_wait.dll"), // Required circa 16.8.3 (14.28.29333)
		vcToolsHostPath.File("tbbmalloc.dll"),            // Required as of 16.2 (14.22.27905)
		vcToolsHostPath.File("vcruntime140.dll"),
		vcToolsHostPath.File("vcruntime140_1.dll")) // Required as of 16.5.1 (14.25.28610)

	if cluiDll, err := vcToolsHostPath.FindFileRec(MakeGlobRegexp("clui.dll")); err == nil {
		x.VcToolsFileSet.Append(
			cluiDll,
			cluiDll.Dirname.File("mspft140ui.dll")) // Localized messages for static analysis
	} else {
		return err
	}

	x.Cl_exe = vcToolsHostPath.File("cl.exe")
	x.Lib_exe = vcToolsHostPath.File("lib.exe")
	x.Link_exe = vcToolsHostPath.File("link.exe")
	if err := bc.NeedFiles(vcToolsVersionFile, x.Cl_exe, x.Lib_exe, x.Link_exe); err != nil {
		return err
	}
	if err := bc.NeedFiles(x.VcToolsFileSet...); err != nil {
		return err
	}
	if err := bc.NeedDirectories(x.VcToolsPath, x.VsInstallPath); err != nil {
		return err
	}

	return nil
}

func (msvc *MsvcCompiler) Build(bc BuildContext) (err error) {

	compileFlags, err := GetCompileFlags(bc)
	if err != nil {
		return err
	}

	if windowsFlags, err := GetWindowsFlags(bc); err == nil {
		msvc.WindowsFlags = *windowsFlags
	} else {
		return err
	}

	msvcProductInstall, err := GetMsvcProductInstall(MsvcProductVer{
		Arch:    msvc.Arch,
		MscVer:  msvc.WindowsFlags.MscVer,
		Insider: msvc.WindowsFlags.Insider,
	}).Need(bc)
	if err != nil {
		return
	}
	msvc.MsvcProductInstall = msvcProductInstall.Alias()

	resourceCompiler, err := GetWindowsResourceCompiler().Need(bc)
	if err != nil {
		return
	}
	msvc.ResourceCompilerInstall = resourceCompiler.Alias()

	windowsSDKInstall, err := GetWindowsSDKInstall(bc, msvc.WindowsFlags.WindowsSDK)
	if err != nil {
		return
	}
	msvc.WindowsSDKInstall = windowsSDKInstall.Alias()

	msc_ver := msvcProductInstall.ActualVer

	msvc.MSC_VER = msc_ver
	msvc.MinorVer = msvcProductInstall.VcToolsPath.Basename()
	msvc.Host = msvcProductInstall.HostArch
	msvc.Target = msvc.Arch.String()
	msvc.PlatformToolset = fmt.Sprintf("%s%s%s", msvc.MinorVer[0:1], msvc.MinorVer[1:2], msvc.MinorVer[3:4])
	msvc.VSInstallName = msvcProductInstall.Selected.InstallationName
	msvc.VSInstallPath = msvcProductInstall.VsInstallPath
	msvc.VCToolsPath = msvcProductInstall.VcToolsPath

	msvc.CompilerRules.CppStd = getCppStdFromMsc(msc_ver)
	msvc.CompilerRules.Features = base.MakeEnumSet(
		COMPILER_ALLOW_CACHING,
		COMPILER_ALLOW_DISTRIBUTION,
		COMPILER_ALLOW_RESPONSEFILE,
		COMPILER_ALLOW_SOURCEMAPPING,
		COMPILER_ALLOW_EDITANDCONTINUE)

	msvc.CompilerRules.Executable = msvcProductInstall.Cl_exe
	msvc.CompilerRules.Librarian = msvcProductInstall.Lib_exe
	msvc.CompilerRules.Linker = msvcProductInstall.Link_exe

	tmpDir := UFS.Transient.Folder("TMP")
	if err := internal_io.CreateDirectory(bc, tmpDir); err != nil {
		return err
	}

	msvc.CompilerRules.Environment = internal_io.NewProcessEnvironment()
	msvc.CompilerRules.Environment.Append("PATH",
		msvcProductInstall.VcToolsHostPath().String(),
		msvcProductInstall.Commond7IdePath().String(),
		resourceCompiler.Executable.Dirname.String(),
		"%PATH%")
	msvc.CompilerRules.Environment.Append("SystemRoot", os.Getenv("SystemRoot"))
	msvc.CompilerRules.Environment.Append("TMP", tmpDir.String())

	msvc.CompilerRules.ExtraFiles = msvcProductInstall.VcToolsFileSet

	msvc.CompilerRules.Facet = NewFacet()
	facet := &msvc.CompilerRules.Facet

	facet.Append(windowsSDKInstall)

	facet.Defines.Append(
		"CPP_VISUALSTUDIO",
		"_ENABLE_EXTENDED_ALIGNED_STORAGE",              // https://devblogs.microsoft.com/cppblog/stl-features-and-fixes-in-vs-2017-15-8/
		"_SILENCE_STDEXT_ARR_ITERS_DEPRECATION_WARNING", // warning STL4043: stdext::checked_array_iterator, stdext::unchecked_array_iterator, and related factory functions are non-Standard extensions and will be removed in the future. std::span (since C++20) and gsl::span can be used instead. You can define _SILENCE_STDEXT_ARR_ITERS_DEPRECATION_WARNING or _SILENCE_ALL_MS_EXT_DEPRECATION_WARNINGS to suppress this warning.
	)

	facet.Exports.Add("VisualStudio/Path", msvc.VSInstallPath.String())
	facet.Exports.Add("VisualStudio/PlatformToolset", msvc.PlatformToolset)
	facet.Exports.Add("VisualStudio/Tools", msvc.VCToolsPath.String())
	facet.Exports.Add("VisualStudio/Version", msvc.MinorVer)

	facet.SystemIncludePaths.Append(
		msvc.VSInstallPath.Folder("VC", "Auxiliary", "VS", "include"),
		msvc.VSInstallPath.Folder("VC", "Tools", "MSVC", msvc.MinorVer, "crt", "src"),
		msvc.VSInstallPath.Folder("VC", "Tools", "MSVC", msvc.MinorVer, "include"))

	facet.AddCompilationFlag_NoAnalysis(
		"/nologo",  // no copyright when compiling
		"/c", "%1", // input file injection
	)

	facet.CompilerOptions.Append("/Fo%2")
	facet.HeaderUnitOptions = base.NewStringSet("/nologo", "/exportHeader", "%1", "/ifcOutput", "%2", "/Fo%3")
	facet.PrecompiledHeaderOptions.Append("/Fp%2", "/Fo%3")
	facet.PreprocessorOptions.Append("/Fo%2")

	facet.AddCompilationFlag(
		"/X",       // ignore standard include paths (we override them with /I)
		"/GF",      // string pooling
		"/GT",      // fiber safe optimizations (https://msdn.microsoft.com/fr-fr/library/6e298fy4.aspx)
		"/bigobj",  // more sections inside obj files, support larger translation units, needed for unity builds
		"/d2FH4",   // https://devblogs.microsoft.com/cppblog/msvc-backend-updates-in-visual-studio-2019-preview-2/
		"/EHsc",    // structure exception support (#TODO: optional ?)
		"/fp:fast", // non-deterministic, allow vendor specific float intrinsics (https://msdn.microsoft.com/fr-fr/library/tzkfha43.aspx)
		"/vmb",     // class is always defined before pointer to member (https://docs.microsoft.com/en-us/cpp/build/reference/vmb-vmg-representation-method?view=vs-2019)
		"/openmp-", // disable OpenMP automatic parallelization
		//"/Za",                // disable non-ANSI features
		"/Zc:inline",           // https://msdn.microsoft.com/fr-fr/library/dn642448.aspx
		"/Zc:implicitNoexcept", // https://msdn.microsoft.com/fr-fr/library/dn818588.aspx
		"/Zc:preprocessor",     // https://devblogs.microsoft.com/cppblog/announcing-full-support-for-a-c-c-conformant-preprocessor-in-msvc/
		"/Zc:rvalueCast",       // https://msdn.microsoft.com/fr-fr/library/dn449507.aspx
		"/Zc:strictStrings",    // https://msdn.microsoft.com/fr-fr/library/dn449508.aspx
		"/Zc:wchar_t",          // promote wchar_t as a native type
		"/Zc:forScope",         // prevent from spilling iterators outside loops
		"/Zc:sizedDealloc",     // https://learn.microsoft.com/en-us/cpp/build/reference/zc-sizeddealloc-enable-global-sized-dealloc-functions?view=msvc-170
		"/Zc:__cplusplus",      // https://docs.microsoft.com/en-us/cpp/build/reference/zc-cplusplus?view=msvc-170
		"/utf-8",               // https://docs.microsoft.com/fr-fr/cpp/build/reference/utf-8-set-source-and-executable-character-sets-to-utf-8
		"/W4",                  // warning level 4 (verbose)
		"/TP",                  // compile as C++
	)

	// ignored warnings
	facet.AddCompilationFlag(
		"/wd4201", // nonstandard extension used: nameless struct/union'
		"/wd4251", // 'XXX' needs to have dll-interface to be used by clients of class 'YYY'
	)

	// configure librarian
	facet.LibrarianOptions.Append(
		"/OUT:%2",
		"%1",
		"/nologo",
		"/SUBSYSTEM:WINDOWS",
		"/IGNORE:4221",
	)

	// configure linker
	facet.LinkerOptions.Append(
		"/OUT:%2",
		"%1",
		"kernel32.lib",
		"Shell32.lib",
		"Gdi32.lib",
		"Advapi32.lib",
		"Shlwapi.lib",
		"Version.lib",
		"/nologo",            // no copyright when compiling
		"/TLBID:1",           // https://msdn.microsoft.com/fr-fr/library/b1kw34cb.aspx
		"/IGNORE:4001",       // https://msdn.microsoft.com/en-us/library/aa234697(v=vs.60).aspx
		"/IGNORE:4099",       // don't have PDB for some externals
		"/NXCOMPAT:NO",       // disable Data Execution Prevention (DEP)
		"/LARGEADDRESSAWARE", // indicate support for VM > 2Gb (if 3Gb flag is toggled)
		"/SUBSYSTEM:WINDOWS", // ~Windows~ application type (vs Console)
		"/fastfail",          // better error reporting
	)

	// strict vs permissive
	if msvc.WindowsFlags.Permissive.Get() {
		base.LogVeryVerbose(LogWindows, "MSVC: using permissive compilation options")

		facet.AddCompilationFlag("/permissive", "/WX-")
		//facet.LinkerOptions.Append("/WX-")
		facet.LibrarianOptions.Append("/WX-")
	} else {
		base.LogVeryVerbose(LogWindows, "MSVC: using strict warnings and warings as error")

		facet.AddCompilationFlag(
			// https://docs.microsoft.com/en-us/cpp/build/reference/permissive-standards-conformance
			"/permissive-",
			// warning as errors
			"/WX",
			// promote some warnings as errors
			"/we4062", // enumerator 'identifier' in a switch of enum 'enumeration' is not handled
			"/we4263", // 'function' : member function does not override any base class virtual member function
			"/we4265", // 'class': class has virtual functions, but destructor is not virtual // not handler by boost and stl
			"/we4296", // 'operator': expression is always false
			"/we4555", // expression has no effect; expected expression with side-effect
			"/we4619", // #pragma warning : there is no warning number 'number'
			"/we4640", // 'instance' : construction of local static object is not thread-safe
			"/we4826", // Conversion from 'type1 ' to 'type_2' is sign-extended. This may cause unexpected runtime behavior.
			"/we4836", // nonstandard extension used : 'type' : local types or unnamed types cannot be used as template arguments
			"/we4905", // wide string literal cast to 'LPSTR'
			"/we4906", // string literal cast to 'LPWSTR'
		)

		// warning as errors also for librarian and linker
		facet.LibrarianOptions.Append("/WX")
		//facet.LinkerOptions.Append("/WX") // #TODO: **DON'T**, will freeze link.exe ¯\_(ツ)_/¯
	}

	if compileFlags.Benchmark.Get() {
		base.LogVeryVerbose(LogWindows, "MSVC: will dump compilation timings")
		facet.CompilerOptions.Append("/d2cgsummary", "/Bt+")
		facet.LinkerOptions.Append("/d2:-cgsummary")
	}

	if msc_ver >= MSC_VER_2019 {
		if msvc.WindowsFlags.JustMyCode.Get() {
			base.LogVeryVerbose(LogWindows, "MSVC: using just-my-code")
			facet.AddCompilationFlag_NoAnalysis("/JMC")
		} else {
			facet.AddCompilationFlag_NoAnalysis("/JMC-")
		}
	}

	if msc_ver >= MSC_VER_2019 {
		base.LogVeryVerbose(LogWindows, "MSCV: using external headers to ignore warnings in foreign code")
		// https://docs.microsoft.com/fr-fr/cpp/build/reference/jmc?view=msvc-160
		facet.Defines.Append("USE_PPE_MSVC_PRAGMA_SYSTEMHEADER")
		facet.AddCompilationFlag_NoAnalysis(
			"/experimental:external",
			"/external:W0",
			"/external:anglebrackets")
	}

	// Windows 10 slow-down workaround
	facet.LinkerOptions.Append(
		"delayimp.lib",
		"/DELAYLOAD:Shell32.dll",
		"/IGNORE:4199", // warning LNK4199: /DELAYLOAD:XXX.dll ignored; no imports found from XXX.dll, caused by our added .libs
	)

	return nil
}

type MsvcProductVer struct {
	Arch    ArchType
	MscVer  MsvcVersion
	Insider BoolVar
}

func GetMsvcProductInstall(prms MsvcProductVer) BuildFactoryTyped[*MsvcProductInstall] {
	return MakeBuildFactory(func(bi BuildInitializer) (MsvcProductInstall, error) {
		if prms.MscVer == MSC_VER_LATEST {
			prms.MscVer = msc_ver_any
		}

		return MsvcProductInstall{
			WantedVer: prms.MscVer,
			Arch:      prms.Arch.String(),
			Insider:   prms.Insider.Get(),
		}, bi.NeedFiles(MSVC_VSWHERE_EXE)
	})
}

func GetMsvcCompiler(arch ArchType) BuildFactoryTyped[*MsvcCompiler] {
	return MakeBuildFactory(func(bi BuildInitializer) (MsvcCompiler, error) {
		msvc := MsvcCompiler{
			Arch:          arch,
			CompilerRules: NewCompilerRules(NewCompilerAlias("msvc", "VisualStudio", arch.String())),
		}
		base.Assert(func() bool {
			var compiler Compiler = &msvc
			return compiler == &msvc
		})
		return msvc, nil
	})
}
