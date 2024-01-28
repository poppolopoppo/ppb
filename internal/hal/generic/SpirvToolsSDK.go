package generic

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"
	"github.com/poppolopoppo/ppb/utils"
)

var SpirvTools = compile.RegisterArchetype("SDK/SPIRV-TOOLS", func(rules *compile.ModuleRules) error {
	rules.Generate(compile.PUBLIC, "spirv-tools-headers.generated.h", &SpirvToolsHeaderGenerator{
		Version: SpirvToolsHeaderVersion,
	})
	return nil
})

/***************************************
 * Spirv-Tools header generator
 ***************************************/

var SpirvToolsHeaderVersion = "SpirvToolsGeneratedHeader-1.0.0"

type SpirvToolsHeaderGenerator struct {
	Version string
}

func (x *SpirvToolsHeaderGenerator) Serialize(ar base.Archive) {
	ar.String(&x.Version)
}
func (x SpirvToolsHeaderGenerator) CreateGenerated(unit *compile.Unit, output utils.Filename) compile.Generated {
	return &SpirvToolsGeneratedHeader{
		Version:     x.Version,
		ExtractDir:  unit.ModuleDir.AbsoluteFolder(unit.Exports.Get("SpirvTools/Path")),
		UseDebugSDK: !unit.Tagged(compile.TAG_NDEBUG),
	}
}

type SpirvToolsGeneratedHeader struct {
	Version     string
	ExtractDir  utils.Directory
	UseDebugSDK bool
}

func (x *SpirvToolsGeneratedHeader) Serialize(ar base.Archive) {
	ar.String(&x.Version)
	ar.Serializable(&x.ExtractDir)
	ar.Bool(&x.UseDebugSDK)
}
func (x SpirvToolsGeneratedHeader) Generate(bc utils.BuildContext, generated *compile.BuildGenerated, dst io.Writer) error {
	config := getSpirvToolsConfig(x.UseDebugSDK)
	downloader, err := getSpirvToolsDownloader(config).Need(bc)
	if err != nil {
		return err
	}

	extractDir := x.ExtractDir.Folder(config)
	ar, err := getSpirvToolsExtractor(downloader, extractDir).Need(bc)
	if err != nil {
		return err
	}

	cpp := internal_io.NewCppFile(dst, false)
	cpp.Comment("SPIRV-Tools header generated by %v - spirvtools v%v", utils.CommandEnv.Prefix(), x.Version)
	cpp.Pragma("once")

	includeDir := ar.Destination.Folder("install", "include")
	includeRe := utils.MakeGlobRegexp(spirvToolsGlobIncludes...)

	for _, x := range ar.ExtractedFiles {
		rel := x.Relative(ar.Destination)
		if includeRe.MatchString(rel) {
			cpp.Pragma("include_alias(\"%v\", \"%v\")",
				utils.SanitizePath(x.Relative(includeDir), '/'),
				utils.SanitizePath(x.Relative(utils.UFS.Source), '/'))
		}
	}

	return nil
}

/***************************************
 * Download spirv-tools release from googleapis
 ***************************************/

var spirvToolsGlobIncludes = base.StringSet{
	"install/include/spirv-tools/*",
}

func getSpirvToolsExtractor(download *internal_io.Downloader, extractDir utils.Directory) utils.BuildFactoryTyped[*internal_io.CompressedUnarchiver] {
	return internal_io.BuildCompressedArchiveExtractorFromDownload(internal_io.CompressedArchiveFromDownload{
		Download:   download,
		ExtractDir: extractDir,
	}, spirvToolsGlobIncludes)
}

const spirvToolsUseFrozenArtifacts = true

// already happened that CI would fail, and not binary was available on HEAD...
var spirvToolsFrozenArtifactUris = map[string]string{
	"windows_vs2017_release": "https://storage.googleapis.com/spirv-tools/artifacts/prod/graphics_shader_compiler/spirv-tools/windows-msvc-2017-release/continuous/1691/20220215-161749/install.zip",
	"windows_vs2017_debug":   "https://storage.googleapis.com/spirv-tools/artifacts/prod/graphics_shader_compiler/spirv-tools/windows-msvc-2017-debug/continuous/1409/20220304-121700/install.zip",
	"linux_clang_release":    "https://storage.googleapis.com/spirv-tools/artifacts/prod/graphics_shader_compiler/spirv-tools/linux-clang-release/continuous/1707/20220304-122028/install.tgz",
	"linux_clang_debug":      "https://storage.googleapis.com/spirv-tools/artifacts/prod/graphics_shader_compiler/spirv-tools/linux-clang-debug/continuous/1718/20220304-122547/install.tgz",
	"macos_clang_release":    "https://storage.googleapis.com/spirv-tools/artifacts/prod/graphics_shader_compiler/spirv-tools/macos-clang-release/continuous/1719/20220304-121705/install.tgz",
	"macos_clang_debug":      "https://storage.googleapis.com/spirv-tools/artifacts/prod/graphics_shader_compiler/spirv-tools/macos-clang-debug/continuous/1725/20220304-121703/install.tgz",
}

func getSpirvToolsConfig(debug bool) string {
	var config string
	switch utils.CurrentHost().Id {
	case base.HOST_WINDOWS:
		config = "windows_vs2017"
	case base.HOST_LINUX:
		config = "linux_clang"
	case base.HOST_DARWIN:
		config = "macos_clang"
	default:
		base.NotImplemented("SPIRV-Tools: no support available for platform '%s'", utils.CurrentHost().Id)
	}
	if debug {
		config += "_debug"
	} else {
		config += "_release"
	}
	return config
}

func getSpirvToolsDownloader(config string) utils.BuildFactoryTyped[*internal_io.Downloader] {
	dst := utils.UFS.Transient.Folder("SDK").File("spirv-tools-master-" + config + ".zip")
	if spirvToolsUseFrozenArtifacts {
		if frozenUrl, ok := spirvToolsFrozenArtifactUris[config]; ok {
			dst = dst.ReplaceExt(filepath.Ext(frozenUrl))
			return internal_io.BuildDownloader(frozenUrl, dst, internal_io.DOWNLOAD_DEFAULT)
		} else {
			base.LogPanic(LogGeneric, "spirv-tools: unknown frozen artifact for <%v>", config)
			return nil
		}
	} else {
		latestUrl := fmt.Sprintf("https://storage.googleapis.com/spirv-tools/badges/build_link_%s.html", config)
		return internal_io.BuildDownloader(latestUrl, dst, internal_io.DOWNLOAD_REDIRECT)
	}
}