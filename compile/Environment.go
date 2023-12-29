package compile

import (
	"fmt"
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
func (x *EnvironmentAlias) AutoComplete(in base.AutoComplete) {
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

func (env *CompileEnv) GetBuildPlatform() (Platform, error) {
	return FindGlobalBuildable[Platform](env.EnvironmentAlias.PlatformAlias.Alias())
}
func (env *CompileEnv) GetBuildConfig() (*BuildConfig, error) {
	return FindGlobalBuildable[*BuildConfig](env.EnvironmentAlias.ConfigurationAlias.Alias())
}
func (env *CompileEnv) GetBuildCompiler() (Compiler, error) {
	return FindGlobalBuildable[Compiler](env.CompilerAlias.Alias())
}

func (env *CompileEnv) GetPlatform() *PlatformRules {
	if platform, err := env.GetBuildPlatform(); err == nil {
		return platform.GetPlatform()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (env *CompileEnv) GetConfig() *ConfigRules {
	if config, err := env.GetBuildConfig(); err == nil {
		return config.GetConfig()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (env *CompileEnv) GetCompiler() *CompilerRules {
	if compiler, err := env.GetBuildCompiler(); err == nil {
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
func (env *CompileEnv) GetCpp(module *ModuleRules) CppRules {
	result := CppRules{}
	result.Inherit((*CppRules)(&env.CompileFlags))

	if module != nil {
		result.Inherit(&module.CppRules)
	}
	if config := env.GetConfig(); config != nil {
		result.Inherit(&env.GetConfig().CppRules)
	}
	if compiler := env.GetCompiler(); compiler != nil {
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
	if compile, err := GetBuildableFlags(GetCompileFlags()).Need(bc); err == nil {
		env.CompileFlags = compile.Flags
	} else {
		return err
	}

	env.CompilerAlias = CompilerAlias{}

	if platform, err := env.GetBuildPlatform(); err == nil {
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
	env.Facet.Append(env.GetPlatform(), env.GetConfig(), env.GetCompiler())

	return nil
}

func GetCompileEnvironment(env EnvironmentAlias) BuildFactoryTyped[*CompileEnv] {
	return MakeBuildFactory(func(bi BuildInitializer) (CompileEnv, error) {
		// register dependency to Configuration/Platform
		// Compiler is a dynamic dependency, since it depends on CompileFlags

		config, err := GetBuildConfig(env.ConfigurationAlias).Need(bi)
		if err != nil {
			return CompileEnv{}, err
		}

		platform, err := GetBuildPlatform(env.PlatformAlias).Need(bi)
		if err != nil {
			return CompileEnv{}, err
		}

		return CompileEnv{
			EnvironmentAlias: NewEnvironmentAlias(platform, config),
			Facet:            NewFacet(),
		}, nil
	})
}

func ForeachEnvironmentAlias(each func(EnvironmentAlias) error) error {
	for _, platformName := range AllPlatforms.Keys() {
		for _, configName := range AllConfigurations.Keys() {
			ea := EnvironmentAlias{
				PlatformAlias:      NewPlatformAlias(platformName),
				ConfigurationAlias: NewConfigurationAlias(configName),
			}
			if err := each(ea); err != nil {
				return err
			}
		}
	}
	return nil
}

func GetEnvironmentAliases() (result []EnvironmentAlias) {
	result = make([]EnvironmentAlias, 0, AllPlatforms.Len()*AllConfigurations.Len())
	for _, platformName := range AllPlatforms.Keys() {
		for _, configName := range AllConfigurations.Keys() {
			result = append(result, EnvironmentAlias{
				PlatformAlias:      NewPlatformAlias(platformName),
				ConfigurationAlias: NewConfigurationAlias(configName),
			})
		}
	}
	return result
}

func ForeachCompileEnvironment(each func(BuildFactoryTyped[*CompileEnv]) error) error {
	return ForeachEnvironmentAlias(func(ea EnvironmentAlias) error {
		return each(GetCompileEnvironment(ea))
	})
}
