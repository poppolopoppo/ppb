package compile

import (
	"fmt"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Plaform Alias
 ***************************************/

type PlatformAlias struct {
	PlatformName string
}

func NewPlatformAlias(platformName string) PlatformAlias {
	return PlatformAlias{PlatformName: platformName}
}
func (x *PlatformAlias) Valid() bool {
	return len(x.PlatformName) > 0
}
func (x *PlatformAlias) Alias() BuildAlias {
	return MakeBuildAlias("Rules", "Platform", x.String())
}
func (x *PlatformAlias) String() string {
	base.Assert(func() bool { return x.Valid() })
	return x.PlatformName
}
func (x *PlatformAlias) Serialize(ar base.Archive) {
	ar.String(&x.PlatformName)
}
func (x PlatformAlias) Compare(o PlatformAlias) int {
	return strings.Compare(x.PlatformName, o.PlatformName)
}
func (x *PlatformAlias) Set(in string) (err error) {
	x.PlatformName = in
	return nil
}
func (x *PlatformAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *PlatformAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x *PlatformAlias) AutoComplete(in base.AutoComplete) {
	AllPlatforms.Range(func(s string, p Platform) error {
		in.Add(p.String(), p.GetPlatform().Alias().String())
		return nil
	})
}

/***************************************
 * Plaform Rules
 ***************************************/

var AllPlatforms base.SharedMapT[string, Platform]

type Platform interface {
	GetCompiler() BuildFactoryTyped[Compiler]
	GetPlatform() *PlatformRules
	Buildable
	fmt.Stringer
}

type PlatformRules struct {
	PlatformAlias PlatformAlias

	Os   string
	Arch ArchType

	Facet
}

func (rules *PlatformRules) String() string {
	return rules.PlatformAlias.String()
}
func (rules *PlatformRules) GetFacet() *Facet {
	return rules.Facet.GetFacet()
}
func (rules *PlatformRules) GetPlatform() *PlatformRules {
	return rules
}
func (rules *PlatformRules) Serialize(ar base.Archive) {
	ar.Serializable(&rules.PlatformAlias)

	ar.String(&rules.Os)
	ar.Serializable(&rules.Arch)

	ar.Serializable(&rules.Facet)
}

func (rules *PlatformRules) Decorate(_ *CompileEnv, unit *Unit) error {
	unit.Facet.Defines.Append("TARGET_PLATFORM=" + rules.Os)
	return nil
}

var Platform_X86 = &PlatformRules{
	PlatformAlias: NewPlatformAlias("x86"),
	Arch:          ARCH_X86,
	Facet: Facet{
		Defines: []string{"ARCH_X86", "ARCH_32BIT"},
	},
}
var Platform_X64 = &PlatformRules{
	PlatformAlias: NewPlatformAlias("x64"),
	Arch:          ARCH_X64,
	Facet: Facet{
		Defines: []string{"ARCH_X64", "ARCH_64BIT"},
	},
}
var Platform_ARM = &PlatformRules{
	PlatformAlias: NewPlatformAlias("arm"),
	Arch:          ARCH_ARM,
	Facet: Facet{
		Defines: []string{"ARCH_ARM", "ARCH_64BIT"},
	},
}

/***************************************
 * Build Platform Factory
 ***************************************/

func (x *PlatformRules) Alias() BuildAlias {
	return x.GetPlatform().PlatformAlias.Alias()
}
func (x *PlatformRules) Build(bc BuildContext) error {
	return nil
}

func GetAllPlatformAliases() (result []PlatformAlias) {
	platforms := AllPlatforms.Values()
	result = make([]PlatformAlias, len(platforms))
	for i, it := range platforms {
		result[i] = it.GetPlatform().PlatformAlias
	}
	return
}

func GetBuildPlatform(platformAlias PlatformAlias) BuildFactoryTyped[Platform] {
	return WrapBuildFactory(func(bi BuildInitializer) (Platform, error) {
		if plaform, ok := AllPlatforms.Get(platformAlias.String()); ok {
			return plaform, nil
		} else {
			return nil, fmt.Errorf("compile: unknown platform name %q", platformAlias.String())
		}
	})
}

func ForeachBuildPlatform(each func(BuildFactoryTyped[Platform]) error) error {
	for _, platformName := range AllPlatforms.Keys() {
		platformAlias := NewPlatformAlias(platformName)
		if err := each(GetBuildPlatform(platformAlias)); err != nil {
			return err
		}
	}
	return nil
}

var GetLocalHostPlatformAlias = base.Memoize(func() PlatformAlias {
	arch := CurrentArch()
	for _, platform := range AllPlatforms.Values() {
		if platform.GetPlatform().Arch == arch {
			return platform.GetPlatform().PlatformAlias
		}
	}
	base.UnreachableCode()
	return PlatformAlias{}
})

func GeLocalHostBuildPlatform() BuildFactoryTyped[Platform] {
	return GetBuildPlatform(GetLocalHostPlatformAlias())
}

func FindPlatform(in string) (result Platform, err error) {
	if platform, ok := AllPlatforms.Get(in); ok {
		return platform, nil
	}

	query := strings.ToLower(in)
	autocomplete := base.NewAutoComplete(in, 3)

	for _, key := range AllPlatforms.Keys() {
		platform, _ := AllPlatforms.Get(key)
		autocomplete.Add(key, platform.GetPlatform().Alias().String())
		if strings.ToLower(key) == query {
			return platform, nil
		}
	}

	return nil, fmt.Errorf("unknown platform %q, did you mean %v?", in, autocomplete.Results())
}
