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
	AdaptiveCache         utils.BoolVar
	ResponseFile          utils.BoolVar
	ShowCmds              utils.BoolVar
	ShowFiles             utils.BoolVar
	ShowOutput            utils.BoolVar
}

func (x *ActionFlags) Flags(cfv utils.CommandFlagsVisitor) {
	cfv.Persistent("AdaptiveCache", "exclude sources from cache when locally modified (requires source control)", &x.AdaptiveCache)
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
	AdaptiveCache: base.INHERITABLE_TRUE,

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
	return base.NewEnumSet(opts...)
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
	// Allow action to use source dependencies parsing to track input files (depends on executable support)
	OPT_ALLOW_SOURCEDEPENDENCIES
	// This action should propagate its input files instead of its own output when tracking inputs (for PCH for instance)
	OPT_PROPAGATE_INPUTS
	// This action should run first when possible, since many tasks can depend on it (for PCH for instance)
	OPT_HIGH_PRIORITY

	OPT_ALLOW_CACHEREADWRITE OptionType = OPT_ALLOW_CACHEREAD | OPT_ALLOW_CACHEWRITE
)

func GetOptionTypes() []OptionType {
	return []OptionType{
		OPT_ALLOW_CACHEREAD,
		OPT_ALLOW_CACHEWRITE,
		OPT_ALLOW_DISTRIBUTION,
		OPT_ALLOW_RELATIVEPATH,
		OPT_ALLOW_RESPONSEFILE,
		OPT_ALLOW_SOURCEDEPENDENCIES,
		OPT_PROPAGATE_INPUTS,
		OPT_HIGH_PRIORITY,
	}
}
func (x OptionType) Ord() int32 { return int32(x) }
func (x OptionType) Mask() int32 {
	return base.EnumBitMask(GetOptionTypes()...)
}
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
	case OPT_ALLOW_SOURCEDEPENDENCIES:
		return "ALLOW_SOURCEDEPENDENCIES"
	case OPT_PROPAGATE_INPUTS:
		return "PROPAGATE_INPUTS"
	case OPT_HIGH_PRIORITY:
		return "HIGH_PRIORITY"
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
	case OPT_ALLOW_SOURCEDEPENDENCIES.String():
		*x = OPT_ALLOW_SOURCEDEPENDENCIES
	case OPT_PROPAGATE_INPUTS.String():
		*x = OPT_PROPAGATE_INPUTS
	case OPT_HIGH_PRIORITY.String():
		*x = OPT_HIGH_PRIORITY
	default:
		err = base.MakeUnexpectedValueError(x, in)
	}
	return err
}
func (x OptionType) Description() string {
	switch x {
	case OPT_ALLOW_CACHEREAD:
		return "allow retrieving build artifacts from action cache"
	case OPT_ALLOW_CACHEWRITE:
		return "allow storing build artifacts in action cache"
	case OPT_ALLOW_DISTRIBUTION:
		return "allow remote distribution of action task"
	case OPT_ALLOW_RELATIVEPATH:
		return "allow converting absolute paths to relative paths"
	case OPT_ALLOW_RESPONSEFILE:
		return "allow using response files when command-line exceeds OS limitations"
	case OPT_ALLOW_SOURCEDEPENDENCIES:
		return "allow tracking of implicit source file dependencies"
	case OPT_PROPAGATE_INPUTS:
		return "dependent tasks will depend on current task inputs instead of output"
	case OPT_HIGH_PRIORITY:
		return "action task will be scheduled to run with higher priority than task without this flag"
	default:
		base.UnexpectedValue(x)
		return ""
	}
}
func (x OptionType) AutoComplete(in base.AutoComplete) {
	for _, it := range GetOptionTypes() {
		in.Add(it.String(), it.Description())
	}
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
