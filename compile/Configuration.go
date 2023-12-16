package compile

import (
	"fmt"
	"strings"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Configuration Alias
 ***************************************/

type ConfigurationAlias struct {
	ConfigName string
}

func NewConfigurationAlias(configName string) ConfigurationAlias {
	return ConfigurationAlias{ConfigName: configName}
}
func (x *ConfigurationAlias) Valid() bool {
	return len(x.ConfigName) > 0
}
func (x *ConfigurationAlias) Alias() BuildAlias {
	return MakeBuildAlias("Rules", "Config", x.String())
}
func (x *ConfigurationAlias) String() string {
	Assert(func() bool { return x.Valid() })
	return x.ConfigName
}
func (x *ConfigurationAlias) Serialize(ar Archive) {
	ar.String(&x.ConfigName)
}
func (x *ConfigurationAlias) Compare(o ConfigurationAlias) int {
	return strings.Compare(x.ConfigName, o.ConfigName)
}
func (x *ConfigurationAlias) Set(in string) (err error) {
	x.ConfigName = in
	return nil
}
func (x *ConfigurationAlias) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *ConfigurationAlias) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}
func (x *ConfigurationAlias) AutoComplete(in AutoComplete) {
	AllConfigurations.Range(func(s string, c Configuration) {
		in.Add(c.String())
	})
}

/***************************************
 * Configuration Rules
 ***************************************/

var AllConfigurations SharedMapT[string, Configuration]

type ConfigRules struct {
	ConfigurationAlias ConfigurationAlias
	ConfigType         ConfigType

	CppRules
	Facet
}

type Configuration interface {
	GetConfig() *ConfigRules
	Serializable
	fmt.Stringer
}

func (rules *ConfigRules) String() string {
	return rules.ConfigurationAlias.String()
}

func (rules *ConfigRules) GetConfig() *ConfigRules {
	return rules
}
func (rules *ConfigRules) GetCpp() *CppRules {
	return rules.CppRules.GetCpp()
}
func (rules *ConfigRules) GetFacet() *Facet {
	return rules.Facet.GetFacet()
}
func (rules *ConfigRules) Serialize(ar Archive) {
	ar.Serializable(&rules.ConfigurationAlias)
	ar.Serializable(&rules.ConfigType)

	ar.Serializable(&rules.CppRules)
	ar.Serializable(&rules.Facet)
}

func (rules *ConfigRules) Decorate(_ *CompileEnv, unit *Unit) error {
	switch unit.Payload {
	case PAYLOAD_HEADERS:
	case PAYLOAD_EXECUTABLE, PAYLOAD_OBJECTLIST, PAYLOAD_STATICLIB:
		unit.Facet.Defines.Append("BUILD_LINK_STATIC")
	case PAYLOAD_SHAREDLIB:
		unit.Facet.Defines.Append("BUILD_LINK_DYNAMIC")
	default:
		UnreachableCode()
	}
	return nil
}

var Configuration_Debug = &ConfigRules{
	ConfigurationAlias: NewConfigurationAlias("Debug"),
	ConfigType:         CONFIG_DEBUG,
	CppRules: CppRules{
		CppRtti:       CPPRTTI_ENABLED,
		DebugSymbols:  DEBUG_EMBEDDED,
		DebugFastLink: INHERITABLE_TRUE,
		Exceptions:    EXCEPTION_ENABLED,
		Link:          LINK_STATIC,
		PCH:           PCH_MONOLITHIC,
		Sanitizer:     SANITIZER_NONE,
		Unity:         UNITY_AUTOMATIC,
		LTO:           INHERITABLE_FALSE,
	},
	Facet: Facet{
		Defines: []string{"DEBUG", "_DEBUG"},
		Tags:    MakeEnumSet(TAG_DEBUG),
	},
}
var Configuration_FastDebug = &ConfigRules{
	ConfigurationAlias: NewConfigurationAlias("FastDebug"),
	ConfigType:         CONFIG_FASTDEBUG,
	CppRules: CppRules{
		CppRtti:       CPPRTTI_ENABLED,
		DebugSymbols:  DEBUG_HOTRELOAD,
		DebugFastLink: INHERITABLE_INHERIT,
		Exceptions:    EXCEPTION_ENABLED,
		Link:          LINK_DYNAMIC,
		PCH:           PCH_MONOLITHIC,
		Sanitizer:     SANITIZER_NONE,
		Unity:         UNITY_DISABLED,
		LTO:           INHERITABLE_FALSE,
		Incremental:   INHERITABLE_TRUE,
	},
	Facet: Facet{
		Defines: []string{"DEBUG", "_DEBUG", "FASTDEBUG"},
		Tags:    MakeEnumSet(TAG_FASTDEBUG, TAG_DEBUG),
	},
}
var Configuration_Devel = &ConfigRules{
	ConfigurationAlias: NewConfigurationAlias("Devel"),
	ConfigType:         CONFIG_DEVEL,
	CppRules: CppRules{
		CppRtti:       CPPRTTI_DISABLED,
		DebugSymbols:  DEBUG_EMBEDDED,
		DebugFastLink: INHERITABLE_FALSE,
		Exceptions:    EXCEPTION_ENABLED,
		Link:          LINK_STATIC,
		PCH:           PCH_MONOLITHIC,
		Sanitizer:     SANITIZER_NONE,
		Unity:         UNITY_AUTOMATIC,
		LTO:           INHERITABLE_INHERIT,
	},
	Facet: Facet{
		Defines: []string{"RELEASE", "NDEBUG"},
		Tags:    MakeEnumSet(TAG_DEVEL, TAG_NDEBUG),
	},
}
var Configuration_Test = &ConfigRules{
	ConfigurationAlias: NewConfigurationAlias("Test"),
	ConfigType:         CONFIG_TEST,
	CppRules: CppRules{
		CppRtti:       CPPRTTI_DISABLED,
		DebugSymbols:  DEBUG_EMBEDDED,
		Exceptions:    EXCEPTION_ENABLED,
		DebugFastLink: INHERITABLE_INHERIT,
		Link:          LINK_STATIC,
		PCH:           PCH_MONOLITHIC,
		Sanitizer:     SANITIZER_NONE,
		Unity:         UNITY_AUTOMATIC,
		LTO:           INHERITABLE_TRUE,
	},
	Facet: Facet{
		Defines: []string{"RELEASE", "NDEBUG", "PROFILING_ENABLED"},
		Tags:    MakeEnumSet(TAG_TEST, TAG_NDEBUG, TAG_PROFILING),
	},
}
var Configuration_Shipping = &ConfigRules{
	ConfigurationAlias: NewConfigurationAlias("Shipping"),
	ConfigType:         CONFIG_SHIPPING,
	CppRules: CppRules{
		CppRtti:       CPPRTTI_DISABLED,
		DebugSymbols:  DEBUG_SYMBOLS,
		DebugFastLink: INHERITABLE_FALSE,
		Exceptions:    EXCEPTION_ENABLED,
		Link:          LINK_STATIC,
		PCH:           PCH_MONOLITHIC,
		Sanitizer:     SANITIZER_NONE,
		Unity:         UNITY_AUTOMATIC,
		LTO:           INHERITABLE_TRUE,
		Deterministic: INHERITABLE_TRUE,
		Incremental:   INHERITABLE_FALSE,
	},
	Facet: Facet{
		Defines: []string{"RELEASE", "NDEBUG", "FINAL_RELEASE"},
		Tags:    MakeEnumSet(TAG_SHIPPING, TAG_NDEBUG),
	},
}

/***************************************
 * Build Configuration Factory
 ***************************************/

type BuildConfig struct {
	Configuration
}

func (x *BuildConfig) Alias() BuildAlias {
	return x.GetConfig().ConfigurationAlias.Alias()
}
func (x *BuildConfig) Build(bc BuildContext) error {
	return nil
}
func (x *BuildConfig) Serialize(ar Archive) {
	SerializeExternal(ar, &x.Configuration)
}

func GetBuildConfig(configAlias ConfigurationAlias) BuildFactoryTyped[*BuildConfig] {
	return WrapBuildFactory(func(bi BuildInitializer) (*BuildConfig, error) {
		if config, ok := AllConfigurations.Get(configAlias.String()); ok {
			return &BuildConfig{config}, nil
		} else {
			return nil, fmt.Errorf("compile: unknown configuration name %q", configAlias.String())
		}
	})
}

func ForeachBuildConfig(each func(BuildFactoryTyped[*BuildConfig]) error) error {
	for _, configName := range AllConfigurations.Keys() {
		configAlias := NewConfigurationAlias(configName)
		if err := each(GetBuildConfig(configAlias)); err != nil {
			return err
		}
	}
	return nil
}

func FindConfiguration(in string) (result Configuration, err error) {
	query := strings.ToLower(in)
	names := AllConfigurations.Keys()
	autocomplete := NewAutoComplete(in)
	for _, name := range names {
		autocomplete.Add(name)
		if strings.ToLower(name) == query {
			result, _ = AllConfigurations.Get(name)
			return
		}
	}
	err = fmt.Errorf("unknown configuration %q, did you mean %q?", in, autocomplete.Results(1)[0])
	return
}
