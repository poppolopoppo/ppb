package generic

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

var LogExternalSDK = base.NewLogCategory("SDK")

var ExternalSDK = compile.RegisterArchetype("SDK/EXTERNAL", func(rules *compile.ModuleRules) error {
	sdkName, err := rules.Exports.Get("SDK/Name")
	if err != nil {
		return err
	}

	var headerName string
	if headerName, err = rules.Exports.Get("SDK/GeneratedHeader"); err != nil {
		headerName = sdkName + "-headers.generated.h"
	}

	rules.Generate(compile.PUBLIC, headerName, &ExternalSDKHeaderGenerator{
		SDKName: sdkName,
		Version: ExternalSDKHeaderVersion,
	})
	return nil
})

/***************************************
 * Generic external SDK header generator
 ***************************************/

var ExternalSDKHeaderVersion = "ExternalSDKdHeader-v1.0.0"

type ExternalSDKHeaderGenerator struct {
	SDKName string
	Version string
}

func setOptionalSdkParam[T any, E interface {
	*T
	Set(string) error
}](unit *compile.Unit, key string, optional *base.Optional[T]) (err error) {
	var in string
	if in, err = unit.Exports.Get(key); err == nil {
		err = base.SetOptional[T, E](in, optional)
	} else {
		*optional = base.NoneOption[T]()
		err = nil
	}
	return
}

var errExternalSDKMissingInput = errors.New("missing external SDK: you must at least provide an external directory or a path to an archive file")
var errExternalSDKMissingExtractDirname = errors.New("missing extraction directory name for external SDK archive")

func (x *ExternalSDKHeaderGenerator) Serialize(ar utils.Archive) {
	ar.String(&x.SDKName)
	ar.String(&x.Version)
}
func (x ExternalSDKHeaderGenerator) CreateGenerated(unit *compile.Unit, output utils.Filename) (compile.Generated, error) {
	result := &ExternalSDKGeneratedHeader{
		SDKName: x.SDKName,
		Version: x.Version,
	}

	// External
	if err := setOptionalSdkParam(unit, "SDK/ExternalDir", &result.ExternalDir); err != nil {
		return nil, err
	}

	// Archive
	if err := setOptionalSdkParam(unit, "SDK/ArchiveFile", &result.ArchiveFile); err != nil {
		return nil, err
	}
	if err := setOptionalSdkParam(unit, "SDK/ArchiveURL", &result.ArchiveURL); err != nil {
		return nil, err
	}
	if err := setOptionalSdkParam(unit, "SDK/FollowRedirect", &result.FollowRedirect); err != nil {
		return nil, err
	}

	// construct absolute path from relative dirname provided (or not in the configuration)
	var extractDirname base.Optional[base.InheritableString]
	if err := setOptionalSdkParam(unit, "SDK/ExtractDirname", &extractDirname); err != nil {
		return nil, err
	}
	if relativeDirname, err := extractDirname.Get(); err == nil {
		result.ExtractDir = base.NewOption(unit.ModuleDir.AbsoluteFolder(relativeDirname.Get()))
	}

	// Headers/Libraries
	if err := setOptionalSdkParam(unit, "SDK/Headers", &result.Headers); err != nil {
		return nil, err
	}
	if err := setOptionalSdkParam(unit, "SDK/Libraries", &result.Libraries); err != nil {
		return nil, err
	}
	if err := setOptionalSdkParam(unit, "SDK/HeadersRegexp", &result.HeadersRegexp); err != nil {
		return nil, err
	}
	if err := setOptionalSdkParam(unit, "SDK/LibrariesRegexp", &result.LibrariesRegexp); err != nil {
		return nil, err
	}

	// Relative include sub-directory in external SDK (defaults to "./include/")
	if err := setOptionalSdkParam(unit, "SDK/IncludeDirname", &result.IncludeDirname); err != nil {
		return nil, err
	}

	// Need either an external path, an archive file or an archive url at least
	if !result.ExternalDir.Valid() &&
		!result.ArchiveFile.Valid() &&
		!result.ArchiveURL.Valid() {
		return nil, errExternalSDKMissingInput
	}

	// Need a destination path if an archive was specified
	if (result.ArchiveFile.Valid() || result.ArchiveURL.Valid()) && !result.ExtractDir.Valid() {
		return nil, errExternalSDKMissingExtractDirname
	}

	return result, nil
}

type ExternalSDKGeneratedHeader struct {
	SDKName string
	Version string

	ExternalDir base.Optional[utils.Directory]

	ExtractDir  base.Optional[utils.Directory]
	ArchiveFile base.Optional[utils.Filename]

	ArchiveURL     base.Optional[base.Url]
	FollowRedirect base.Optional[base.InheritableBool]

	Headers         base.Optional[base.StringSet]
	Libraries       base.Optional[base.StringSet]
	HeadersRegexp   base.Optional[base.Regexp]
	LibrariesRegexp base.Optional[base.Regexp]

	IncludeDirname base.Optional[base.InheritableString]
}

func (x *ExternalSDKGeneratedHeader) Serialize(ar utils.Archive) {
	ar.String(&x.SDKName)
	ar.String(&x.Version)
	base.SerializeOptional(ar, &x.ExternalDir)
	base.SerializeOptional(ar, &x.ExtractDir)
	base.SerializeOptional(ar, &x.ArchiveFile)
	base.SerializeOptional(ar, &x.ArchiveURL)
	base.SerializeOptional(ar, &x.FollowRedirect)
	base.SerializeOptional(ar, &x.Headers)
	base.SerializeOptional(ar, &x.Libraries)
	base.SerializeOptional(ar, &x.HeadersRegexp)
	base.SerializeOptional(ar, &x.LibrariesRegexp)
	base.SerializeOptional(ar, &x.IncludeDirname)
}

func (x *ExternalSDKGeneratedHeader) getArchiveAcceptList() base.Regexp {
	var includeDirnameRe base.Regexp

	// Use relative include dir for pattern matching if no explicit headers nor header regexp has been provided
	if includeDirname, err := x.IncludeDirname.Get(); err == nil && !x.Headers.Valid() && !x.HeadersRegexp.Valid() {
		includeDirnameRe = utils.MakeGlobRegexp(includeDirname.Get() + "*")
	}

	return x.LibrariesRegexp.GetOrElse(base.Regexp{}).Concat(
		x.HeadersRegexp.GetOrElse(base.Regexp{})).Concat(
		includeDirnameRe)
}

func (x *ExternalSDKGeneratedHeader) prepareSDKDir(bc utils.BuildContext) (utils.Directory, utils.FileSet, error) {

	if sdkDir, err := x.ExternalDir.Get(); err == nil {
		base.LogVerbose(LogExternalSDK, "%s: using external directory %q for external SDK", x.SDKName, sdkDir)

		// External SDK stored at designated location
		sdkFiles, err := internal_io.ListDirectory(bc, sdkDir)
		return sdkDir, sdkFiles, err

	} else if extractDir, err := x.ExtractDir.Get(); err == nil {

		// External SDK stored in a compresed archive
		if sdkUrl, err := x.ArchiveURL.Get(); err == nil {
			base.LogVerbose(LogExternalSDK, "%s: using HTTP link %q to download compressed external SDK", x.SDKName, sdkUrl)

			// Create a subdirectory from archive name, in case multiple variants are declared (eg Debug vs Release)
			basename := strings.TrimSuffix(path.Base(sdkUrl.Path), path.Ext(sdkUrl.Path))
			extractDir = extractDir.Folder(basename)

			// Must download archive from a remote http(s) server
			downloadMode := internal_io.DOWNLOAD_DEFAULT
			if x.FollowRedirect.GetOrElse(base.INHERITABLE_FALSE).Get() {
				downloadMode = internal_io.DOWNLOAD_REDIRECT
			}

			if downloader, err := internal_io.BuildDownloader(sdkUrl,
				utils.UFS.Transient.Folder("SDK").Folder(x.SDKName).Folder(basename),
				downloadMode).Need(bc); err == nil {

				if extractor, err := internal_io.BuildCompressedArchiveExtractorFromDownload(
					internal_io.CompressedArchiveFromDownload{
						Download:   downloader,
						ExtractDir: extractDir,
					},
					x.getArchiveAcceptList()).Need(bc); err == nil {
					return extractor.Destination, extractor.ExtractedFiles, nil

				} else {
					return utils.Directory{}, utils.FileSet{}, err
				}
			} else {
				return utils.Directory{}, utils.FileSet{}, err
			}

		} else if sdkArchive, err := x.ArchiveFile.Get(); err == nil {
			base.LogVerbose(LogExternalSDK, "%s: using compressed archive %q for external SDK", x.SDKName, sdkArchive)

			// Create a subdirectory from archive name, in case multiple variants are declared (eg Debug vs Release)
			basename := strings.TrimSuffix(sdkArchive.Basename, path.Ext(sdkArchive.Basename))
			extractDir = extractDir.Folder(basename)

			// Archive already stored at designated location
			if extractor, err := internal_io.BuildCompressedUnarchiver(
				sdkArchive, extractDir,
				x.getArchiveAcceptList()).Need(bc); err == nil {
				return extractor.Destination, extractor.ExtractedFiles, nil

			} else {
				return utils.Directory{}, utils.FileSet{}, err
			}
		}
	}
	return utils.Directory{}, utils.FileSet{}, errExternalSDKMissingInput
}

func (x *ExternalSDKGeneratedHeader) Generate(bc utils.BuildContext, generated *compile.BuildGenerated, dst io.Writer) error {
	sdkDir, sdkFiles, err := x.prepareSDKDir(bc)
	if err != nil {
		return err
	}

	var includeDir utils.Directory
	if relativeDir, err := x.IncludeDirname.Get(); err == nil {
		includeDir = sdkDir.AbsoluteFolder(relativeDir.Get())
	} else {
		includeDir = sdkDir.Folder("include")
	}

	var sdkHeaders, sdkLibraries utils.FileSet
	if relative, err := x.Headers.Get(); err == nil {
		for _, it := range relative {
			if header := sdkDir.AbsoluteFile(it); sdkFiles.Contains(header) {
				sdkHeaders.Append(header)
			} else {
				return fmt.Errorf("could not find this SDK header: %q", header)
			}
		}
	}
	if relative, err := x.Libraries.Get(); err == nil {
		for _, it := range relative {
			if lib := sdkDir.AbsoluteFile(it); sdkFiles.Contains(lib) {
				sdkLibraries.Append(lib)
			} else {
				return fmt.Errorf("could not find this SDK library: %q", lib)
			}
		}
	}
	if rexp, err := x.HeadersRegexp.Get(); err == nil {
		for _, it := range sdkFiles {
			if rexp.MatchString(it.Relative(sdkDir)) {
				sdkHeaders.AppendUniq(it)
			}
		}
	}
	if rexp, err := x.LibrariesRegexp.Get(); err == nil {
		for _, it := range sdkFiles {
			if rexp.MatchString(it.Relative(sdkDir)) {
				sdkLibraries.AppendUniq(it)
			}
		}
	}

	base.LogVerbose(LogExternalSDK, "%s: found %d SDK header files (include directory is %q)", x.SDKName, len(sdkHeaders), includeDir)
	if base.IsLogLevelActive(base.LOG_VERYVERBOSE) {
		for _, it := range sdkHeaders {
			base.LogVeryVerbose(LogExternalSDK, "%s: found SDK header %q", x.SDKName, it.Relative(includeDir))
		}
	}

	base.LogVerbose(LogExternalSDK, "%s: found %d SDK library files %q", x.SDKName, len(sdkLibraries))
	if base.IsLogLevelActive(base.LOG_VERYVERBOSE) {
		for _, it := range sdkLibraries {
			base.LogVeryVerbose(LogExternalSDK, "%s: found SDK library %q", x.SDKName, it)
		}
	}

	cpp := internal_io.NewCppFile(dst, false)
	cpp.Comment("External SDK header generated by %v -- %v -- %v", utils.CommandEnv.Prefix(), x.SDKName, x.Version)
	cpp.Pragma("once")

	for _, it := range sdkHeaders {
		cpp.Pragma("include_alias(\"%v\", \"%v\")",
			utils.SanitizePath(it.Relative(includeDir), '/'),
			utils.SanitizePath(it.Relative(utils.UFS.Source), '/'))
	}

	for _, it := range sdkLibraries {
		cpp.Pragma("comment(lib, %q)", it)
	}

	return nil
}
