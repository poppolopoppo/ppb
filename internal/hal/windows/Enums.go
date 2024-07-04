//go:build windows

package windows

import (
	"fmt"
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
 * MSCV Version
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

func getCppStdFromMsc(msc_ver MsvcVersion) compile.CppStdType {
	switch msc_ver {
	case MSC_VER_2022:
		return compile.CPPSTD_20
	case MSC_VER_2019:
		return compile.CPPSTD_17
	case MSC_VER_2017:
		return compile.CPPSTD_14
	case MSC_VER_2015:
		return compile.CPPSTD_14
	case MSC_VER_2013:
		return compile.CPPSTD_11
	default:
		base.UnexpectedValue(msc_ver)
		return compile.CPPSTD_17
	}
}
