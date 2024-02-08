package utils

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
)

/***************************************
 * Build Alias
 ***************************************/

type BuildAlias struct {
	Domain string
	Name   string
}

const BUILD_DOMAIN_SEPARATOR = `://`

type BuildAliases = base.SetT[BuildAlias]

type BuildAliasable interface {
	Alias() BuildAlias
}

func MakeBuildAliases[T BuildAliasable](targets ...T) (result BuildAliases) {
	result = make(BuildAliases, len(targets))
	for i, it := range targets {
		result[i] = it.Alias()
	}
	return
}

func MakeBuildAlias(domain string, names ...string) BuildAlias {
	nameSet := base.StringSet(names)

	var builder BuildAliasBuilder
	MakeBuildAliasBuilder(&builder, domain, nameSet.TotalContentLen()+nameSet.Len() /* separators */)

	for _, it := range nameSet {
		if len(it) == 0 {
			continue
		}
		builder.WriteString('/', it)
	}

	return builder.Alias()
}
func (x BuildAlias) Alias() BuildAlias { return x }
func (x BuildAlias) Valid() bool {
	base.AssertNotIn(len(x.Domain), 0)
	return len(x.Name) > 0
}
func (x BuildAlias) String() string {
	return fmt.Sprint(x.Domain, BUILD_DOMAIN_SEPARATOR, x.Name)
}
func (x BuildAlias) Equals(o BuildAlias) bool {
	return x.Name == o.Name && x.Domain == o.Domain
}
func (x BuildAlias) Compare(o BuildAlias) int {
	if cmp := strings.Compare(x.Domain, o.Domain); cmp != 0 {
		return cmp
	} else {
		return strings.Compare(x.Name, o.Name)
	}
}
func (x BuildAlias) GetHashValue(basis uint64) uint64 {
	return base.Fnv1a(x.Name, base.Fnv1a(x.Domain, basis))
}
func (x *BuildAlias) Set(in string) error {
	parts := strings.SplitN(in, BUILD_DOMAIN_SEPARATOR, 2)
	if len(parts) == 2 {
		x.Domain, x.Name = parts[0], parts[1]
		return nil
	}
	return fmt.Errorf("invalid build alias: %q", in)
}
func (x *BuildAlias) Serialize(ar base.Archive) {
	ar.String(&x.Domain)
	ar.String(&x.Name)
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

/***************************************
 * Build Alias Builder
 ***************************************/

type BuildAliasBuilder struct {
	domain string
	sb     strings.Builder
}

func MakeBuildAliasBuilder(result *BuildAliasBuilder, domain string, capacity int) {
	result.domain = domain
	result.sb.Grow(capacity + 10 /* additional reserve for multi-byte chars */)
}
func (x *BuildAliasBuilder) Alias() BuildAlias {
	return BuildAlias{
		Domain: x.domain,
		Name:   x.sb.String()}
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
		if x.sb.Len() > 0 {
			x.sb.WriteRune(sep)
		}
		BuildSanitizedPath(&x.sb, it, '/')
	}
}
