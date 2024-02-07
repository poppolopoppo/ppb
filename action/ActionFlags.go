package action

import (
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * ActionFlags
 ***************************************/

type ActionFlags struct {
	CacheCompression      base.CompressionFormat
	CacheCompressionLevel base.CompressionLevel
	CacheMode             CacheModeType
	CachePath             utils.Directory
	DistMode              DistModeType
	ResponseFile          utils.BoolVar
	ShowCmds              utils.BoolVar
	ShowFiles             utils.BoolVar
	ShowOutput            utils.BoolVar
}

func (x *ActionFlags) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Persistent("CacheMode", "use input hashing to store/retrieve action outputs", &x.CacheMode)
	cfv.Persistent("CachePath", "set path used to store cached actions", &x.CachePath)
	cfv.Persistent("CacheCompression", "set compression format for cached bulk entries", &x.CacheCompression)
	cfv.Persistent("CacheCompressionLevel", "set compression level for cached bulk entries", &x.CacheCompressionLevel)
	cfv.Persistent("DistMode", "distribute actions to a cluster of remote workers", &x.DistMode)
	cfv.Persistent("ResponseFile", "control response files usage", &x.ResponseFile)
	cfv.Variable("ShowCmds", "print executed compilation commands", &x.ShowCmds)
	cfv.Variable("ShowFiles", "print file accesses for external commands", &x.ShowFiles)
	cfv.Variable("ShowOutput", "always show compilation commands output", &x.ShowOutput)
}

var GetActionFlags = utils.NewCommandParsableFlags(&ActionFlags{
	CacheMode: CACHE_NONE,
	CachePath: utils.UFS.Cache,
	// Lz4 is almost as fast as uncompressed, but with fewer IO: when using Fast speed it is almost always a free win
	CacheCompression:      base.COMPRESSION_FORMAT_LZ4,
	CacheCompressionLevel: base.COMPRESSION_LEVEL_FAST,

	DistMode: DIST_NONE,

	ResponseFile: base.INHERITABLE_TRUE,
	ShowCmds:     base.INHERITABLE_FALSE,
	ShowFiles:    base.INHERITABLE_FALSE,
	ShowOutput:   base.INHERITABLE_FALSE,
})

/***************************************
 * Action Options
 ***************************************/

type OptionType byte
type OptionFlags = base.EnumSet[OptionType, *OptionType]

func MakeOptionFlags(opts ...OptionType) OptionFlags {
	return base.MakeEnumSet[OptionType, *OptionType](opts...)
}

const (
	// Allow action output to be retrieved from cache
	OPT_ALLOW_CACHEREAD OptionType = iota
	// Allow action output to be stored in cache
	OPT_ALLOW_CACHEWRITE
	// Allow action to be distributed in remote peers cluster
	OPT_ALLOW_DISTRIBUTION
	// Allow action to use relative paths (for caching)
	OPT_ALLOW_RELATIVEPATH
	// Allow action to use response files when command-line is too long (depends on executable support)
	OPT_ALLOW_RESPONSEFILE
	// Allow action to check source control for local modications, and avoid storing in cache when dirty
	OPT_ALLOW_SOURCECONTROL
	// This action should propagate its input files instead of its own output when tracking inputs (for PCH)
	OPT_PROPAGATE_INPUTS

	OPT_ALLOW_CACHEREADWRITE OptionType = OPT_ALLOW_CACHEREAD | OPT_ALLOW_CACHEWRITE
)

func OptionTypes() []OptionType {
	return []OptionType{
		OPT_ALLOW_CACHEREAD,
		OPT_ALLOW_CACHEWRITE,
		OPT_ALLOW_DISTRIBUTION,
		OPT_ALLOW_RELATIVEPATH,
		OPT_ALLOW_RESPONSEFILE,
		OPT_ALLOW_SOURCECONTROL,
		OPT_PROPAGATE_INPUTS,
	}
}
func (x OptionType) Ord() int32           { return int32(x) }
func (x *OptionType) FromOrd(value int32) { *x = OptionType(value) }
func (x OptionType) String() string {
	switch x {
	case OPT_ALLOW_CACHEREAD:
		return "ALLOW_CACHEREAD"
	case OPT_ALLOW_CACHEWRITE:
		return "ALLOW_CACHEWRITE"
	case OPT_ALLOW_DISTRIBUTION:
		return "ALLOW_DISTRIBUTION"
	case OPT_ALLOW_RELATIVEPATH:
		return "ALLOW_RELATIVEPATH"
	case OPT_ALLOW_RESPONSEFILE:
		return "ALLOW_RESPONSEFILE"
	case OPT_ALLOW_SOURCECONTROL:
		return "ALLOW_SOURCECONTROL"
	case OPT_PROPAGATE_INPUTS:
		return "PROPAGATE_INPUTS"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x *OptionType) Set(in string) (err error) {
	switch strings.ToUpper(in) {
	case OPT_ALLOW_CACHEREAD.String():
		*x = OPT_ALLOW_CACHEREAD
	case OPT_ALLOW_CACHEWRITE.String():
		*x = OPT_ALLOW_CACHEWRITE
	case OPT_ALLOW_DISTRIBUTION.String():
		*x = OPT_ALLOW_DISTRIBUTION
	case OPT_ALLOW_RELATIVEPATH.String():
		*x = OPT_ALLOW_RELATIVEPATH
	case OPT_ALLOW_RESPONSEFILE.String():
		*x = OPT_ALLOW_RESPONSEFILE
	case OPT_ALLOW_SOURCECONTROL.String():
		*x = OPT_ALLOW_SOURCECONTROL
	case OPT_PROPAGATE_INPUTS.String():
		*x = OPT_PROPAGATE_INPUTS
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x *OptionType) Serialize(ar base.Archive) {
	ar.Byte((*byte)(x))
}
func (x OptionType) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *OptionType) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
