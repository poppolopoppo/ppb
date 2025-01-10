package compile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

/***************************************
 * Environment Alias
 ***************************************/

type EnvironmentAlias struct {
	PlatformAlias
	ConfigurationAlias
}

func NewEnvironmentAlias(platform Platform, config Configuration) EnvironmentAlias {
	return EnvironmentAlias{
		PlatformAlias:      platform.GetPlatform().PlatformAlias,
		ConfigurationAlias: config.GetConfig().ConfigurationAlias,
	}
}
func (x *EnvironmentAlias) Valid() bool {
	return x.PlatformAlias.Valid() && x.ConfigurationAlias.Valid()
}
func (x *EnvironmentAlias) Alias() BuildAlias {
	return MakeBuildAlias("Rules", "Environment", x.PlatformName, x.ConfigName)
}
func (x EnvironmentAlias) String() string {
	base.Assert(func() bool { return x.Valid() })
	return fmt.Sprintf("%v-%v", x.PlatformName, x.ConfigName)
}
func (x *EnvironmentAlias) Serialize(ar base.Archive) {
	ar.Serializable(&x.PlatformAlias)
	ar.Serializable(&x.ConfigurationAlias)
}
func (x EnvironmentAlias) Compare(o EnvironmentAlias) int {
	if cmp := x.PlatformAlias.Compare(o.PlatformAlias); cmp == 0 {
		return x.ConfigurationAlias.Compare(o.ConfigurationAlias)
	} else {
		return cmp
	}
}
func (x *EnvironmentAlias) Set(in string) error {
	if _, err := fmt.Sscanf(in, "%s-%s", &x.PlatformName, &x.ConfigName); err == nil {
		if err := x.PlatformAlias.Set(x.PlatformName); err != nil {
			return err
		}
		if err := x.ConfigurationAlias.Set(x.ConfigName); err != nil {
			return err
		}
		return nil
	} else {
		return err
	}
}
func (x *EnvironmentAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *EnvironmentAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}
func (x EnvironmentAlias) AutoComplete(in base.AutoComplete) {
	ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		in.Add(ea.String(), ea.Alias().String())
		return nil
	})
}

/***************************************
 * Compilation Environment
 ***************************************/

type CompileEnv struct {
	EnvironmentAlias EnvironmentAlias
	Facet

	CompilerAlias CompilerAlias
	CompileFlags  CompileFlags
}

func (env *CompileEnv) Family() []string {
	return []string{env.EnvironmentAlias.PlatformName, env.EnvironmentAlias.ConfigName}
}
func (env *CompileEnv) String() string {
	return strings.Join(append([]string{env.CompilerAlias.CompilerName}, env.Family()...), "_")
}
func (env *CompileEnv) Serialize(ar base.Archive) {
	ar.Serializable(&env.EnvironmentAlias)
	ar.Serializable(&env.Facet)
	ar.Serializable(&env.CompilerAlias)
	SerializeParsableFlags(ar, &env.CompileFlags)
}

func (env *CompileEnv) GetBuildPlatform(bg BuildGraphReadPort) (Platform, error) {
	return FindBuildable[Platform](bg, env.EnvironmentAlias.PlatformAlias.Alias())
}
func (env *CompileEnv) GetBuildConfig(bg BuildGraphReadPort) (*BuildConfig, error) {
	return FindBuildable[*BuildConfig](bg, env.EnvironmentAlias.ConfigurationAlias.Alias())
}
func (env *CompileEnv) GetBuildCompiler(bg BuildGraphReadPort) (Compiler, error) {
	return FindBuildable[Compiler](bg, env.CompilerAlias.Alias())
}

func (env *CompileEnv) GetPlatform(bg BuildGraphReadPort) *PlatformRules {
	if platform, err := env.GetBuildPlatform(bg); err == nil {
		return platform.GetPlatform()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (env *CompileEnv) GetConfig(bg BuildGraphReadPort) *ConfigRules {
	if config, err := env.GetBuildConfig(bg); err == nil {
		return config.GetConfig()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (env *CompileEnv) GetCompiler(bg BuildGraphReadPort) *CompilerRules {
	if compiler, err := env.GetBuildCompiler(bg); err == nil {
		return compiler.GetCompiler()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}

func (env *CompileEnv) GetFacet() *Facet { return &env.Facet }

func (env *CompileEnv) GeneratedDir() Directory {
	return UFS.Generated.Folder(env.Family()...)
}
func (env *CompileEnv) IntermediateDir() Directory {
	return UFS.Intermediate.Folder(env.Family()...)
}
func (env *CompileEnv) GetCpp(bg BuildGraphReadPort, module *ModuleRules) CppRules {
	result := CppRules{}

	if module != nil {
		result.Inherit(&module.CppRules)
	}

	result.Inherit((*CppRules)(&env.CompileFlags))

	if config := env.GetConfig(bg); config != nil {
		result.Inherit(&env.GetConfig(bg).CppRules)
	}
	if compiler := env.GetCompiler(bg); compiler != nil {
		base.Inherit(&result.CppStd, compiler.CppStd)
	}

	return result
}
func (env *CompileEnv) GetPayloadType(module *ModuleRules, link LinkType) (result PayloadType) {
	switch module.ModuleType {
	case MODULE_EXTERNAL:
		switch module.Link {
		case LINK_INHERIT:
			fallthrough
		case LINK_STATIC:
			return PAYLOAD_OBJECTLIST
		case LINK_DYNAMIC:
			return PAYLOAD_SHAREDLIB
		default:
			base.UnexpectedValuePanic(module.ModuleType, link)
		}
	case MODULE_LIBRARY:
		switch link {
		case LINK_INHERIT:
			fallthrough
		case LINK_STATIC:
			return PAYLOAD_STATICLIB
		case LINK_DYNAMIC:
			return PAYLOAD_SHAREDLIB
		default:
			base.UnexpectedValuePanic(module.ModuleType, link)
		}
	case MODULE_PROGRAM:
		switch link {
		case LINK_INHERIT:
			fallthrough
		case LINK_STATIC:
			return PAYLOAD_EXECUTABLE
		case LINK_DYNAMIC:
			base.LogPanic(LogCompile, "executable should have %s link, but found %s", LINK_STATIC, link)
		default:
			base.UnexpectedValuePanic(module.ModuleType, link)
		}
	case MODULE_HEADERS:
		return PAYLOAD_HEADERS
	default:
		base.UnexpectedValuePanic(module.ModuleType, module.ModuleType)
	}
	return result
}

func (env *CompileEnv) ModuleAlias(module Module) TargetAlias {
	return TargetAlias{
		EnvironmentAlias: env.EnvironmentAlias,
		ModuleAlias:      module.GetModule().ModuleAlias,
	}
}

/***************************************
 * Compilation Environment Factory
 ***************************************/

func (env *CompileEnv) Alias() BuildAlias {
	return env.EnvironmentAlias.Alias()
}
func (env *CompileEnv) Build(bc BuildContext) error {
	env.CompilerAlias = CompilerAlias{}

	if compileFlags, err := GetCompileFlags(bc); err == nil {
		env.CompileFlags = *compileFlags
	} else {
		return err
	}

	if platform, err := env.GetBuildPlatform(bc); err == nil {
		if compiler, err := platform.GetCompiler().Need(bc); err == nil {
			env.CompilerAlias = compiler.GetCompiler().CompilerAlias
		} else {
			return err
		}
	} else {
		return err
	}

	env.Facet = NewFacet()
	env.Facet.Defines.Append(
		"BUILD_ENVIRONMENT="+env.String(),
		"BUILD_PLATFORM="+env.EnvironmentAlias.PlatformName,
		"BUILD_CONFIG="+env.EnvironmentAlias.ConfigName,
		"BUILD_COMPILER="+env.CompilerAlias.String(),
		"BUILD_FAMILY="+strings.Join(env.Family(), "-"),
		"BUILD_"+strings.Join(env.Family(), "_"))

	env.Facet.IncludePaths.Append(UFS.Source)
	env.Facet.Append(env.GetPlatform(bc), env.GetConfig(bc), env.GetCompiler(bc))

	return nil
}

func GetCompileEnvironment(ea EnvironmentAlias) BuildFactoryTyped[*CompileEnv] {
	return MakeBuildFactory(func(bi BuildInitializer) (CompileEnv, error) {
		// register dependency to Configuration/Platform
		// Compiler is a dynamic dependency, since it depends on CompileFlags

		return CompileEnv{
				EnvironmentAlias: ea,
				Facet:            NewFacet(),
			}, bi.NeedFactories(
				GetBuildPlatform(ea.PlatformAlias),
				GetBuildConfig(ea.ConfigurationAlias),
			)
	})
}

func ForeachEnvironmentAlias(each func(EnvironmentAlias) error) error {
	plaformNames := AllPlatforms.Keys()
	configNames := AllConfigurations.Keys()

	sort.Strings(plaformNames)
	sort.Strings(configNames)

	for _, platformName := range plaformNames {
		for _, configName := range configNames {
			if err := each(EnvironmentAlias{
				PlatformAlias:      NewPlatformAlias(platformName),
				ConfigurationAlias: NewConfigurationAlias(configName),
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

func GetEnvironmentAliases() (results []EnvironmentAlias) {
	results = make([]EnvironmentAlias, 0, AllPlatforms.Len()*AllConfigurations.Len())
	err := ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		results = append(results, ea)
		return nil
	})
	base.LogPanicIfFailed(LogCompile, err)
	return
}

func ForeachCompileEnvironment(each func(BuildFactoryTyped[*CompileEnv]) error) error {
	return ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		return each(GetCompileEnvironment(ea))
	})
}
