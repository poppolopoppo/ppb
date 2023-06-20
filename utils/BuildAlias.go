package utils

import (
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * Build Alias
 ***************************************/

type BuildAlias string
type BuildAliases = base.SetT[BuildAlias]

type BuildAliasable interface {
	Alias() BuildAlias
}

func MakeBuildAliases[T BuildAliasable](targets ...T) (result BuildAliases) {
	result = make(BuildAliases, len(targets))

	for i, it := range targets {
		result[i] = it.Alias()
	}

	return result
}

func ConcatBuildAliases[T BuildAliasable](targets ...[]T) (result BuildAliases) {
	capacity := 0
	for _, arr := range targets {
		capacity += len(arr)
	}

	result = make(BuildAliases, capacity)

	i := 0
	for _, arr := range targets {
		for _, it := range arr {
			result[i] = it.Alias()
			i++
		}
	}

	return result
}

func FindBuildAliases(bg BuildGraph, category string, names ...string) (result BuildAliases) {
	prefix := MakeBuildAlias(category, names...).String()
	for _, a := range bg.Aliases() {
		if strings.HasPrefix(a.String(), prefix) {
			result.Append(a)
		}
	}
	return
}

type BuildAliasBuilder struct {
	sb strings.Builder
}

func MakeBuildAliasBuilder(result *BuildAliasBuilder, category string, capacity int) {
	result.sb.Grow(capacity + len(category) + len(":/") + 10 /* additional reserve for multi-byte chars */)
	result.sb.WriteString(category)
	result.sb.WriteString(":/")
}
func (x *BuildAliasBuilder) Alias() BuildAlias {
	return BuildAlias(x.sb.String())
}
func (x *BuildAliasBuilder) ReserveString(strs ...string) {
	capacity := 0
	for _, it := range strs {
		capacity += 1 + len(it)
	}
	x.sb.Grow(capacity)
}
func (x *BuildAliasBuilder) WriteString(sep rune, strs ...string) {
	for _, it := range strs {
		x.sb.WriteRune(sep)
		BuildSanitizedPath(&x.sb, it, '/')
	}
}

func MakeBuildAlias(category string, names ...string) BuildAlias {
	sb := strings.Builder{}
	sep := "://"

	capacity := len(category)
	i := 0
	for _, it := range names {
		if len(it) == 0 {
			continue
		}
		if i > 0 {
			capacity++
		} else {
			capacity += len(sep)
		}
		capacity += len(it)
		i++
	}
	sb.Grow(capacity)

	sb.WriteString(category)
	i = 0
	for _, it := range names {
		if len(it) == 0 {
			continue
		}
		if i > 0 {
			sb.WriteRune('/')
		} else {
			sb.WriteString(sep)
		}
		BuildSanitizedPath(&sb, it, '/')
		i++
	}

	return BuildAlias(sb.String())
}
func (x BuildAlias) Alias() BuildAlias { return x }
func (x BuildAlias) Valid() bool       { return len(x) > 3 /* check for "---" */ }
func (x BuildAlias) Equals(o BuildAlias) bool {
	return (string)(x) == (string)(o)
}
func (x BuildAlias) Compare(o BuildAlias) int {
	return strings.Compare((string)(x), (string)(o))
}
func (x BuildAlias) String() string {
	base.Assert(func() bool { return x.Valid() })
	return (string)(x)
}
func (x *BuildAlias) Set(in string) error {
	base.Assert(func() bool { return x.Valid() })
	*x = BuildAlias(in)
	return nil
}
func (x *BuildAlias) Serialize(ar base.Archive) {
	ar.String((*string)(x))
}
func (x *BuildAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *BuildAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x BuildAlias) AutoComplete(in base.AutoComplete) {
	bg := CommandEnv.BuildGraph()
	for _, a := range bg.Aliases() {
		in.Add(a.String(), "")
	}
}
