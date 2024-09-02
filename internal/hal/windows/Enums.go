//go:build windows

package windows

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/poppolopoppo/ppb/compile"
	"github.com/poppolopoppo/ppb/internal/base"
)

type CompilerType byte

const (
	COMPILER_MSVC CompilerType = iota
	COMPILER_CLANGCL
)

func GetCompilerTypes() []CompilerType {
	return []CompilerType{
		COMPILER_MSVC,
		COMPILER_CLANGCL,
	}
}
func (x CompilerType) Description() string {
	switch x {
	case COMPILER_MSVC:
		return "Microsoft Visual C++ compiler"
	case COMPILER_CLANGCL:
		return "LLVM Clang C++ compiler"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x CompilerType) String() string {
	switch x {
	case COMPILER_MSVC:
		return "MSVC"
	case COMPILER_CLANGCL:
		return "CLANGCL"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *CompilerType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case COMPILER_MSVC.String():
		*x = COMPILER_MSVC
	case COMPILER_CLANGCL.String():
		*x = COMPILER_CLANGCL
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *CompilerType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x CompilerType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *CompilerType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *CompilerType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetCompilerTypes() {
		in.Add(it.String(), it.Description())
	}
}

/***************************************
 * MSVC PlatformToolset
 ***************************************/

type MsvcPlatformToolset byte

const (
	MSVC_PLATFORMTOOLSET_142 MsvcPlatformToolset = 142
	MSVC_PLATFORMTOOLSET_143 MsvcPlatformToolset = 143
	MSVC_PLATFORMTOOLSET_144 MsvcPlatformToolset = 144
)

func GetMsvcPlatformToolsets() []MsvcPlatformToolset {
	return []MsvcPlatformToolset{
		MSVC_PLATFORMTOOLSET_142,
		MSVC_PLATFORMTOOLSET_143,
		MSVC_PLATFORMTOOLSET_144,
	}
}
func (v MsvcPlatformToolset) Equals(o MsvcPlatformToolset) bool {
	return (v == o)
}
func (v MsvcPlatformToolset) String() string {
	switch v {
	case MSVC_PLATFORMTOOLSET_144:
		return "144"
	case MSVC_PLATFORMTOOLSET_143:
		return "143"
	case MSVC_PLATFORMTOOLSET_142:
		return "143"
	default:
		return strconv.FormatUint(uint64(byte(v)), 10)
	}
}
func (v *MsvcPlatformToolset) Set(in string) (err error) {
	switch in {
	case MSVC_PLATFORMTOOLSET_144.String():
		*v = MSVC_PLATFORMTOOLSET_144
	case MSVC_PLATFORMTOOLSET_143.String():
		*v = MSVC_PLATFORMTOOLSET_143
	case MSVC_PLATFORMTOOLSET_142.String():
		*v = MSVC_PLATFORMTOOLSET_142
	default:
		base.LogWarningOnce(LogWindows, "unknown MSVC platform toolset: %q", in)
		var long uint64
		long, err = strconv.ParseUint(in, 10, 8)
		if err == nil {
			*v = MsvcPlatformToolset((byte)(long))
		}
	}
	return err
}
func (x *MsvcPlatformToolset) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x *MsvcPlatformToolset) AutoComplete(in base.AutoComplete) {
	for _, it := range GetMsvcPlatformToolsets() {
		in.Add(it.String(), fmt.Sprint("Microsoft Visual Studio ", it.String()))
	}
}

func getPlatformToolsetFromMinorVer(minorVer string) (result MsvcPlatformToolset, err error) {
	err = result.Set(fmt.Sprintf("%s%s%s", minorVer[0:1], minorVer[1:2], minorVer[3:4]))
	if err == nil {
		if result == MSVC_PLATFORMTOOLSET_144 && strings.HasPrefix(minorVer, "14.40.") {
			// https://devblogs.microsoft.com/cppblog/msvc-toolset-minor-version-number-14-40-in-vs-2022-v17-10/
			base.LogWarningOnce(LogWindows, "MSVC still expects platform toolset v143 instead of v144, insteadf MSVC toolset version must be used to select v144 toolchain")
			result = MSVC_PLATFORMTOOLSET_143
		}
	}
	return
}

/***************************************
 * MSVC Version
 ***************************************/

type MsvcVersion int32

const (
	msc_ver_any    MsvcVersion = -1
	MSC_VER_LATEST MsvcVersion = 0
	MSC_VER_2022   MsvcVersion = 1930
	MSC_VER_2019   MsvcVersion = 1920
	MSC_VER_2017   MsvcVersion = 1910
	MSC_VER_2015   MsvcVersion = 1900
	MSC_VER_2013   MsvcVersion = 1800
)

func GetMsvcVersions() []MsvcVersion {
	return []MsvcVersion{
		MSC_VER_2022,
		MSC_VER_2019,
		MSC_VER_2017,
		MSC_VER_2015,
		MSC_VER_2013,
	}
}
func (v MsvcVersion) Equals(o MsvcVersion) bool {
	return (v == o)
}
func (v MsvcVersion) String() string {
	switch v {
	case msc_ver_any:
		return "ANY"
	case MSC_VER_LATEST:
		return "LATEST"
	case MSC_VER_2022:
		return "2022"
	case MSC_VER_2019:
		return "2019"
	case MSC_VER_2017:
		return "2017"
	case MSC_VER_2015:
		return "2015"
	case MSC_VER_2013:
		return "2013"
	default:
		base.UnreachableCode()
		return ""
	}
}
func (v *MsvcVersion) Set(in string) (err error) {
	switch in {
	case msc_ver_any.String():
		*v = msc_ver_any
	case MSC_VER_LATEST.String():
		*v = MSC_VER_LATEST
	case MSC_VER_2022.String():
		*v = MSC_VER_2022
	case MSC_VER_2019.String():
		*v = MSC_VER_2019
	case MSC_VER_2017.String():
		*v = MSC_VER_2017
	case MSC_VER_2015.String():
		*v = MSC_VER_2015
	case MSC_VER_2013.String():
		*v = MSC_VER_2013
	default:
		err = base.MakeUnexpectedValueError(v, in)
	}
	return err
}
func (x *MsvcVersion) Serialize(ar base.Archive) {
	ar.Int32((*int32)(x))
}
func (x *MsvcVersion) AutoComplete(in base.AutoComplete) {
	for _, it := range GetMsvcVersions() {
		in.Add(it.String(), fmt.Sprint("Microsoft Visual Studio ", it.String()))
	}
}

func getCppStdFromMsc(msc_ver MsvcVersion) (compile.CppStdType, error) {
	switch msc_ver {
	case MSC_VER_2022:
		return compile.CPPSTD_23, nil
	case MSC_VER_2019:
		return compile.CPPSTD_17, nil
	case MSC_VER_2017:
		return compile.CPPSTD_14, nil
	case MSC_VER_2015:
		return compile.CPPSTD_14, nil
	case MSC_VER_2013:
		return compile.CPPSTD_11, nil
	default:
		return compile.CPPSTD_INHERIT, base.MakeUnexpectedValueError(compile.CPPSTD_INHERIT, msc_ver)
	}
}
