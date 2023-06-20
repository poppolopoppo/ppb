package windows

import (
	"regexp"
	"sort"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

type WindowsSDK struct {
	Name             string
	RootDir          utils.Directory
	Version          string
	ResourceCompiler utils.Filename
	compile.Facet
}

func newWindowsSDK(rootDir utils.Directory, version string) (result WindowsSDK) {
	result = WindowsSDK{
		Name:             "WindowsSDK_" + version,
		RootDir:          rootDir,
		Version:          version,
		ResourceCompiler: rootDir.Folder("bin", version, "x64").File("rc.exe"),
		Facet:            compile.NewFacet(),
	}
	result.Facet.LibraryPaths.Append(
		rootDir.Folder("Lib", version, "ucrt", "{{.Windows/Platform}}"),
		rootDir.Folder("Lib", version, "um", "{{.Windows/Platform}}"),
	)
	result.Facet.SystemIncludePaths.Append(
		rootDir.Folder("Include", version, "ucrt"),
		rootDir.Folder("Include", version, "um"),
		rootDir.Folder("Include", version, "shared"),
	)
	result.Facet.Defines.Append(
		"STRICT",                   // https://msdn.microsoft.com/en-us/library/windows/desktop/aa383681(v=vs.85).aspx
		"NOMINMAX",                 // https://support.microsoft.com/en-us/kb/143208
		"VC_EXTRALEAN",             // https://support.microsoft.com/en-us/kb/166474
		"WIN32_LEAN_AND_MEAN",      // https://support.microsoft.com/en-us/kb/166474
		"_NO_W32_PSEUDO_MODIFIERS", // Prevent windows from #defining IN or OUT (undocumented)
		// "DBGHELP_TRANSLATE_TCHAR",  // https://msdn.microsoft.com/en-us/library/windows/desktop/ms679294(v=vs.85).aspx
		"_UNICODE",          // https://msdn.microsoft.com/fr-fr/library/dybsewaf.aspx
		"UNICODE",           // defaults to UTF-8
		"_HAS_EXCEPTIONS=0", // Disable STL exceptions
		"OEMRESOURCE",       // https://docs.microsoft.com/en-us/windows/desktop/api/winuser/nf-winuser-setsystemcursor
	)
	return result
}

func (sdk *WindowsSDK) GetFacet() *compile.Facet {
	return &sdk.Facet
}
func (sdk *WindowsSDK) Serialize(ar base.Archive) {
	ar.String(&sdk.Name)
	ar.Serializable(&sdk.RootDir)
	ar.String(&sdk.Version)
	ar.Serializable(&sdk.ResourceCompiler)
	ar.Serializable(&sdk.Facet)
}

type WindowsSDKInstall struct {
	MajorVer   string
	SearchDir  utils.Directory
	SearchGlob string
	WindowsSDK
}

func (x *WindowsSDKInstall) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("HAL", "Windows", "SDK", x.MajorVer)
}
func (x *WindowsSDKInstall) Build(bc utils.BuildContext) error {
	var dirs utils.DirSet
	var err error
	if x.MajorVer != "User" {
		err = x.SearchDir.MatchDirectories(func(d utils.Directory) error {
			dirs.Append(d)
			return nil
		}, regexp.MustCompile(x.SearchGlob))
	} else {
		windowsFlags := GetWindowsFlags()
		if _, err = utils.GetBuildableFlags(windowsFlags).Need(bc); err != nil {
			return err
		}

		dirs.Append(windowsFlags.WindowsSDK)
		_, err = windowsFlags.WindowsSDK.Info()
	}
	if err == nil && len(dirs) > 0 {
		sort.Sort(dirs)
		lib := dirs[len(dirs)-1]
		if err = bc.NeedDirectory(lib); err != nil {
			return err
		}

		base.LogDebug(LogWindows, "found WindowsSDK@%v in '%v'", x.MajorVer, lib)

		libParent, ver := lib.Split()
		x.WindowsSDK = newWindowsSDK(libParent.Parent(), ver)
		err = bc.NeedFile(x.WindowsSDK.ResourceCompiler)
	}
	return err
}
func (x *WindowsSDKInstall) Serialize(ar base.Archive) {
	ar.String(&x.MajorVer)
	ar.Serializable(&x.SearchDir)
	ar.String(&x.SearchGlob)
	ar.Serializable(&x.WindowsSDK)
}

func getWindowsSDK_10() utils.BuildFactoryTyped[*WindowsSDKInstall] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (WindowsSDKInstall, error) {
		searchDir := utils.MakeDirectory("C:/Program Files (x86)/Windows Kits/10/Lib")
		return WindowsSDKInstall{
			MajorVer:   "10",
			SearchDir:  searchDir,
			SearchGlob: `10\..*`,
		}, bi.NeedDirectory(searchDir)
	})
}

func getWindowsSDK_8_1() utils.BuildFactoryTyped[*WindowsSDKInstall] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (WindowsSDKInstall, error) {
		searchDir := utils.MakeDirectory("C:/Program Files (x86)/Windows Kits/8.1/Lib")
		return WindowsSDKInstall{
			MajorVer:   "8.1",
			SearchDir:  searchDir,
			SearchGlob: `8\..*`,
		}, bi.NeedDirectory(searchDir)
	})
}

func getWindowsSDK_User(overrideDir utils.Directory) utils.BuildFactoryTyped[*WindowsSDKInstall] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (WindowsSDKInstall, error) {
		return WindowsSDKInstall{
			MajorVer:  "User",
			SearchDir: overrideDir,
		}, bi.NeedDirectory(overrideDir)
	})
}

func GetWindowsSDKInstall(bi utils.BuildInitializer, overrideDir utils.Directory) (*WindowsSDKInstall, error) {
	if len(overrideDir.Path) > 0 {
		base.LogPanicIfFailed(LogWindows, bi.NeedDirectory(overrideDir))

		base.LogVeryVerbose(LogWindows, "using user override '%v' for Windows SDK", overrideDir)
		return getWindowsSDK_User(overrideDir).Need(bi)
	}

	if win10, err := getWindowsSDK_10().Need(bi); err == nil {
		base.LogVeryVerbose(LogWindows, "using Windows SDK 10")
		return win10, nil
	}

	if win81, err := getWindowsSDK_8_1().Need(bi); err == nil {
		base.LogVeryVerbose(LogWindows, "using Windows SDK 8.1")
		return win81, nil
	} else {
		return nil, err
	}
}
