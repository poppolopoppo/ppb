package windows

import (
	"regexp"
	"sort"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/compile"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

type WindowsSDK struct {
	Name             string
	RootDir          Directory
	Version          string
	ResourceCompiler Filename
	Facet
}

func newWindowsSDK(rootDir Directory, version string) (result WindowsSDK) {
	result = WindowsSDK{
		Name:             "WindowsSDK_" + version,
		RootDir:          rootDir,
		Version:          version,
		ResourceCompiler: rootDir.Folder("Bin", version, "x64").File("RC.exe"),
		Facet:            NewFacet(),
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

func (sdk *WindowsSDK) GetFacet() *Facet {
	return &sdk.Facet
}
func (sdk *WindowsSDK) Serialize(ar Archive) {
	ar.String(&sdk.Name)
	ar.Serializable(&sdk.RootDir)
	ar.String(&sdk.Version)
	ar.Serializable(&sdk.ResourceCompiler)
	ar.Serializable(&sdk.Facet)
}

type WindowsSDKInstall struct {
	MajorVer   string
	SearchDir  Directory
	SearchGlob string
	WindowsSDK
}

func (x *WindowsSDKInstall) Alias() BuildAlias {
	return MakeBuildAlias("HAL", "Windows", "SDK", x.MajorVer)
}
func (x *WindowsSDKInstall) Build(bc BuildContext) error {
	var dirs DirSet
	var err error
	if x.MajorVer != "User" {
		err = x.SearchDir.MatchDirectories(func(d Directory) error {
			dirs.Append(d)
			return nil
		}, regexp.MustCompile(x.SearchGlob))
	} else {
		windowsFlags := GetWindowsFlags()
		if _, err = GetBuildableFlags(windowsFlags).Need(bc); err != nil {
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

		LogDebug(LogWindows, "found WindowsSDK@%v in '%v'", x.MajorVer, lib)

		libParent, ver := lib.Split()
		x.WindowsSDK = newWindowsSDK(libParent.Parent(), ver)
		err = bc.NeedFile(x.WindowsSDK.ResourceCompiler)
	}
	return err
}
func (x *WindowsSDKInstall) Serialize(ar Archive) {
	ar.String(&x.MajorVer)
	ar.Serializable(&x.SearchDir)
	ar.String(&x.SearchGlob)
	ar.Serializable(&x.WindowsSDK)
}

func getWindowsSDK_10() BuildFactoryTyped[*WindowsSDKInstall] {
	return MakeBuildFactory(func(bi BuildInitializer) (WindowsSDKInstall, error) {
		searchDir := MakeDirectory("C:/Program Files (x86)/Windows Kits/10/Lib")
		return WindowsSDKInstall{
			MajorVer:   "10",
			SearchDir:  searchDir,
			SearchGlob: `10\..*`,
		}, bi.NeedDirectory(searchDir)
	})
}

func getWindowsSDK_8_1() BuildFactoryTyped[*WindowsSDKInstall] {
	return MakeBuildFactory(func(bi BuildInitializer) (WindowsSDKInstall, error) {
		searchDir := MakeDirectory("C:/Program Files (x86)/Windows Kits/8.1/Lib")
		return WindowsSDKInstall{
			MajorVer:   "8.1",
			SearchDir:  searchDir,
			SearchGlob: `8\..*`,
		}, bi.NeedDirectory(searchDir)
	})
}

func getWindowsSDK_User(overrideDir Directory) BuildFactoryTyped[*WindowsSDKInstall] {
	return MakeBuildFactory(func(bi BuildInitializer) (WindowsSDKInstall, error) {
		return WindowsSDKInstall{
			MajorVer:  "User",
			SearchDir: overrideDir,
		}, bi.NeedDirectory(overrideDir)
	})
}

func GetWindowsSDKInstall(bi BuildInitializer, overrideDir Directory) (*WindowsSDKInstall, error) {
	if len(overrideDir.Path) > 0 {
		LogPanicIfFailed(LogWindows, bi.NeedDirectory(overrideDir))

		LogVeryVerbose(LogWindows, "using user override '%v' for Windows SDK", overrideDir)
		return getWindowsSDK_User(overrideDir).Need(bi)
	}

	if win10, err := getWindowsSDK_10().Need(bi); err == nil {
		LogVeryVerbose(LogWindows, "using Windows SDK 10")
		return win10, nil
	}

	if win81, err := getWindowsSDK_8_1().Need(bi); err == nil {
		LogVeryVerbose(LogWindows, "using Windows SDK 8.1")
		return win81, nil
	} else {
		return nil, err
	}
}
