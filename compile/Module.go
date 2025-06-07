package compile

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"

	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

type Module interface {
	GetModule() *ModuleRules
	GetNamespace(bg BuildGraphReadPort) *NamespaceRules
	ExpandModule(env *CompileEnv) ModuleRules
	Buildable
	base.Serializable
	fmt.Stringer
}

/***************************************
 * Module Arche Type
 ***************************************/

var AllArchetypes base.SharedMapT[string, ModuleArchetype]

type ModuleArchetype func(*ModuleRules) error

func RegisterArchetype(archtype string, fn ModuleArchetype) ModuleArchetype {
	archtype = strings.ToUpper(archtype)
	AllArchetypes.Add(archtype, fn)
	return fn
}

var ErrUnknownModuleArchtype = errors.New("unknown module archetype")

type ModuleArchetypeAliases = base.SetT[ModuleArchetypeAlias]

type ModuleArchetypeAlias struct {
	ArchetypeName string
}

func (x ModuleArchetypeAlias) String() string {
	return x.ArchetypeName
}
func (x *ModuleArchetypeAlias) Serialize(ar base.Archive) {
	ar.String(&x.ArchetypeName)
}
func (x ModuleArchetypeAlias) Compare(o ModuleArchetypeAlias) int {
	return strings.Compare(x.ArchetypeName, o.ArchetypeName)
}
func (x ModuleArchetypeAlias) AutoComplete(in base.AutoComplete) {
	AllArchetypes.Range(func(s string, ma ModuleArchetype) error {
		in.Add(s, "")
		return nil
	})
}
func (x *ModuleArchetypeAlias) Set(in string) (err error) {
	archtype := strings.ToUpper(in)
	if _, ok := AllArchetypes.Get(archtype); ok {
		x.ArchetypeName = archtype
		return nil
	}
	return ErrUnknownModuleArchtype
}
func (x *ModuleArchetypeAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ModuleArchetypeAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}

/***************************************
 * Module Alias
 ***************************************/

type ModuleAlias struct {
	NamespaceAlias
	ModuleName string
}

type ModuleAliases = base.SetT[ModuleAlias]

func NewModuleAlias(namespace Namespace, moduleName string) ModuleAlias {
	return ModuleAlias{
		NamespaceAlias: namespace.GetNamespace().NamespaceAlias,
		ModuleName:     moduleName,
	}
}
func (x ModuleAlias) Valid() bool {
	return x.NamespaceAlias.Valid() && len(x.ModuleName) > 0
}
func (x ModuleAlias) Alias() BuildAlias {
	return MakeBuildAlias("Rules", "Module", x.NamespaceName, x.ModuleName)
}
func (x ModuleAlias) String() string {
	return path.Join(x.NamespaceAlias.String(), x.ModuleName)
}
func (x *ModuleAlias) Serialize(ar base.Archive) {
	ar.Serializable(&x.NamespaceAlias)
	ar.String(&x.ModuleName)
}
func (x ModuleAlias) Compare(o ModuleAlias) int {
	namespaceCmp := x.NamespaceAlias.Compare(o.NamespaceAlias)
	switch namespaceCmp {
	case 0:
		return strings.Compare(x.ModuleName, o.ModuleName)
	default:
		return namespaceCmp
	}
}
func (x ModuleAlias) AutoComplete(in base.AutoComplete) {
	if bg, ok := in.GetUserParam().(BuildGraphReadPort); ok {
		ForeachBuildable(bg, func(_ BuildAlias, m Module) error {
			it := m.GetModule().ModuleAlias
			in.Add(it.String(), m.GetModule().ModuleType.String())
			return nil
		})
	} else {
		base.UnreachableCode()
	}
}
func (x *ModuleAlias) Set(in string) (err error) {
	if parts := SplitPath(in); len(parts) > 1 {
		x.ModuleName = parts[len(parts)-1]
		return x.NamespaceAlias.Set(path.Join(parts[0 : len(parts)-1]...))
	}
	return fmt.Errorf("malformed ModuleAlias: '%s'", in)
}
func (x *ModuleAlias) MarshalText() ([]byte, error) {
	return base.UnsafeBytesFromString(x.String()), nil
}
func (x *ModuleAlias) UnmarshalText(data []byte) error {
	return x.Set(base.UnsafeStringFromBytes(data))
}

/***************************************
 * Module Source
 ***************************************/

type ModuleSource struct {
	SourceDirs    DirSet
	SourceGlobs   base.StringSet
	ExcludedGlobs base.StringSet
	SourceFiles   FileSet
	ExcludedFiles FileSet
	IsolatedFiles FileSet
	ExtraFiles    FileSet
	ExtraDirs     DirSet
}

func (x *ModuleSource) Append(o ModuleSource) {
	x.SourceDirs.Append(o.SourceDirs...)
	x.SourceGlobs.Append(o.SourceGlobs...)
	x.ExcludedGlobs.Append(o.ExcludedGlobs...)
	x.SourceFiles.Append(o.SourceFiles...)
	x.ExcludedFiles.Append(o.ExcludedFiles...)
	x.IsolatedFiles.Append(o.IsolatedFiles...)
	x.ExtraFiles.Append(o.ExtraFiles...)
	x.ExtraDirs.Append(o.ExtraDirs...)
}
func (x *ModuleSource) Prepend(o ModuleSource) {
	x.SourceDirs.Prepend(o.SourceDirs...)
	x.SourceGlobs.Prepend(o.SourceGlobs...)
	x.ExcludedGlobs.Prepend(o.ExcludedGlobs...)
	x.SourceFiles.Prepend(o.SourceFiles...)
	x.ExcludedFiles.Prepend(o.ExcludedFiles...)
	x.IsolatedFiles.Prepend(o.IsolatedFiles...)
	x.ExtraFiles.Prepend(o.ExtraFiles...)
	x.ExtraDirs.Prepend(o.ExtraDirs...)
}
func (x *ModuleSource) Serialize(ar base.Archive) {
	ar.Serializable(&x.SourceDirs)
	ar.Serializable(&x.SourceGlobs)
	ar.Serializable(&x.ExcludedGlobs)
	ar.Serializable(&x.SourceFiles)
	ar.Serializable(&x.ExcludedFiles)
	ar.Serializable(&x.IsolatedFiles)
	ar.Serializable(&x.ExtraFiles)
	ar.Serializable(&x.ExtraDirs)
}
func (x *ModuleSource) GetFileSet(bc BuildContext) (FileSet, error) {
	result := FileSet{}

	for _, source := range x.SourceDirs {
		if files, err := internal_io.GlobDirectory(bc, source, x.SourceGlobs, x.ExcludedGlobs, x.ExcludedFiles); err == nil {
			result.AppendUniq(files...)
		} else {
			return FileSet{}, err
		}
	}

	result.AppendUniq(x.SourceFiles...)
	result.AppendUniq(x.IsolatedFiles...)

	// result.AppendUniq(x.ExtraFiles...) // voluntary ignore ExtraDirs/ExtraFiles here
	return result, nil
}

/***************************************
 * Module Rules
 ***************************************/

type ModuleRules struct {
	ModuleAlias ModuleAlias

	ModuleDir  Directory
	ModuleType ModuleType

	CppRules

	PrecompiledHeader Filename
	PrecompiledSource Filename

	PublicDependencies  ModuleAliases
	PrivateDependencies ModuleAliases
	RuntimeDependencies ModuleAliases

	Customs    CustomList
	Generators GeneratorList

	Facet
	Source ModuleSource

	PerTags map[TagFlags]ModuleRules
}

func (rules *ModuleRules) GetModule() *ModuleRules {
	return rules
}

func (rules *ModuleRules) GetBuildNamespace(bg BuildGraphReadPort) (Namespace, error) {
	return FindBuildNamespace(bg, rules.ModuleAlias.NamespaceAlias)
}
func (rules *ModuleRules) GetNamespace(bg BuildGraphReadPort) *NamespaceRules {
	if namespace, err := rules.GetBuildNamespace(bg); err == nil {
		return namespace.GetNamespace()
	} else {
		base.LogPanicErr(LogCompile, err)
		return nil
	}
}
func (rules *ModuleRules) String() string {
	return rules.ModuleAlias.String()
}

func (rules *ModuleRules) RelativePath() string {
	return rules.ModuleDir.Relative(UFS.Source)
}
func (rules *ModuleRules) PublicDir() Directory {
	return rules.ModuleDir.Folder("Public")
}
func (rules *ModuleRules) PrivateDir() Directory {
	return rules.ModuleDir.Folder("Private")
}
func (rules *ModuleRules) GeneratedDir(env *CompileEnv) Directory {
	return env.GeneratedDir().AbsoluteFolder(rules.RelativePath())
}

func (rules *ModuleRules) GetCpp() *CppRules {
	return rules.CppRules.GetCpp()
}
func (rules *ModuleRules) GetFacet() *Facet {
	return rules.Facet.GetFacet()
}

func (x *ModuleRules) DeepCopy(src *ModuleRules) {
	// first, copy by value
	*x = *src

	// then, clone reference values
	x.CppRules.DeepCopy(&src.CppRules)

	x.PublicDependencies = base.CopySlice(src.PublicDependencies...)
	x.PrivateDependencies = base.CopySlice(src.PrivateDependencies...)
	x.RuntimeDependencies = base.CopySlice(src.RuntimeDependencies...)

	x.Customs = base.CopySlice(src.Customs...)
	x.Generators = base.CopySlice(src.Generators...)

	x.Facet.DeepCopy(&src.Facet)

	x.PerTags = base.CopyMap(x.PerTags)

}

func (rules *ModuleRules) expandTagsRec(env *CompileEnv, dst *ModuleRules) {
	for tags, tagged := range rules.PerTags {
		if selectedTags := env.Tags.Intersect(tags); !selectedTags.Empty() {
			base.LogVeryVerbose(LogCompile, "expand module %q with rules tagged [%v]", dst.ModuleAlias, selectedTags)
			dst.Prepend(&tagged)
			tagged.expandTagsRec(env, dst)
		}
	}
}
func (rules *ModuleRules) ExpandModule(env *CompileEnv) ModuleRules {
	// first, create a deep copy module rules
	var expanded ModuleRules
	expanded.DeepCopy(rules)

	// we use this getter to create new rules and apply PerTags properties
	if env != nil && len(rules.PerTags) > 0 {
		// apply tags matching compile env recursively
		rules.expandTagsRec(env, &expanded)
	}

	// always return a copy: rules should not be modified outside of Build()
	return expanded
}

func (rules *ModuleRules) Decorate(bg BuildGraphReadPort, env *CompileEnv, unit *Unit) error {
	if err := rules.GetNamespace(bg).Decorate(bg, env, unit); err != nil {
		return err
	}

	// do not make force includes transitives
	// unit.TransitiveFacet.ForceIncludes.Append(rules.ForceIncludes...)
	unit.TransitiveFacet.Libraries.Append(rules.Libraries...)
	unit.TransitiveFacet.LibraryPaths.Append(rules.LibraryPaths...)

	if publicDir := rules.PublicDir(); publicDir.Exists() {
		unit.IncludePaths.Append(publicDir)
		unit.TransitiveFacet.IncludePaths.Append(publicDir)
	}
	if privateDir := rules.PrivateDir(); privateDir.Exists() {
		unit.IncludePaths.Append(privateDir)
	}

	var generatedVis base.EnumSet[VisibilityType, *VisibilityType]
	for _, gen := range rules.Generators {
		generatedVis.Add(gen.GetGenerator().Visibility)
	}
	if generatedVis.Has(PUBLIC) {
		generatedPublicDir := unit.GeneratedDir.Folder("Public")
		unit.IncludePaths.Append(generatedPublicDir)
		unit.TransitiveFacet.IncludePaths.Append(generatedPublicDir)
	}
	if generatedVis.Has(PRIVATE) {
		unit.IncludePaths.Append(unit.GeneratedDir.Folder("Private"))
	}

	unit.IncludePaths.Append(rules.ModuleDir)
	return nil
}

func (rules *ModuleRules) Serialize(ar base.Archive) {
	ar.Serializable(&rules.ModuleAlias)

	ar.Serializable(&rules.ModuleDir)
	ar.Serializable(&rules.ModuleType)

	ar.Serializable(&rules.CppRules)

	ar.Serializable(&rules.PrecompiledHeader)
	ar.Serializable(&rules.PrecompiledSource)

	base.SerializeSlice(ar, rules.PublicDependencies.Ref())
	base.SerializeSlice(ar, rules.PrivateDependencies.Ref())
	base.SerializeSlice(ar, rules.RuntimeDependencies.Ref())

	ar.Serializable(&rules.Customs)
	ar.Serializable(&rules.Generators)

	ar.Serializable(&rules.Facet)
	ar.Serializable(&rules.Source)

	base.SerializeMap(ar, &rules.PerTags)
}

func (rules *ModuleRules) Generate(vis VisibilityType, name string, gen Generator) {
	rules.Generators.Append(GeneratorRules{
		GeneratedName: name,
		Visibility:    vis,
		Generator:     gen,
	})
}

func (x *ModuleRules) Append(other *ModuleRules) {
	x.CppRules.Inherit(other.GetCpp())

	x.ForceIncludes.Append(other.ForceIncludes...)

	x.Source.Append(other.Source)

	if !x.PrecompiledHeader.Valid() {
		x.PrecompiledHeader = other.PrecompiledHeader
	}
	if !x.PrecompiledSource.Valid() {
		x.PrecompiledSource = other.PrecompiledSource
	}

	x.PrivateDependencies.Append(other.PrivateDependencies...)
	x.PublicDependencies.Append(other.PublicDependencies...)
	x.RuntimeDependencies.Append(other.RuntimeDependencies...)

	x.Customs.Append(other.Customs...)
	x.Generators.Append(other.Generators...)

	x.Facet.Append(other)
}
func (x *ModuleRules) Prepend(other *ModuleRules) {
	x.Overwrite(other.GetCpp())

	x.ForceIncludes.Prepend(other.ForceIncludes...)

	x.Source.Prepend(other.Source)

	if other.PrecompiledHeader.Valid() {
		x.PrecompiledHeader = other.PrecompiledHeader
	}
	if other.PrecompiledSource.Valid() {
		x.PrecompiledSource = other.PrecompiledSource
	}

	x.PrivateDependencies.Prepend(other.PrivateDependencies...)
	x.PublicDependencies.Prepend(other.PublicDependencies...)
	x.RuntimeDependencies.Prepend(other.RuntimeDependencies...)

	x.Customs.Prepend(other.Customs...)
	x.Generators.Prepend(other.Generators...)

	x.Facet.Prepend(other)
}

/***************************************
 * Build Module
 ***************************************/

func (x *ModuleRules) Alias() BuildAlias {
	return x.ModuleAlias.Alias()
}
func (x *ModuleRules) Build(bc BuildContext) error {
	ForeachCompileEnvironment(func(bft BuildFactoryTyped[*CompileEnv]) error {
		_, err := bc.OutputFactory(WrapBuildFactory(func(bi BuildInitializer) (*Unit, error) {
			compileEnv, err := bft.Need(bi)
			if err != nil {
				return nil, err
			}

			return &Unit{
				TargetAlias: TargetAlias{
					ModuleAlias:      x.ModuleAlias,
					EnvironmentAlias: compileEnv.EnvironmentAlias,
				},
			}, nil
		}))
		return err
	})
	return nil
}

func FindBuildModule(bg BuildGraphReadPort, module ModuleAlias) (Module, error) {
	return FindBuildable[Module](bg, module.Alias())
}

func NeedBuildModules(bc BuildContext, moduleAliases ...ModuleAlias) (modules []Module, err error) {
	if err = bc.DependsOn(base.Map(func(ma ModuleAlias) BuildAlias {
		return MakeBuildAlias("Model", ma.String())
	}, moduleAliases...)...); err != nil {
		return
	}

	modules = make([]Module, len(moduleAliases))

	for i, moduleAlias := range moduleAliases {
		var buildable Buildable
		if buildable, err = bc.NeedBuildable(moduleAlias); err != nil {
			return
		}

		modules[i] = buildable.(Module)
	}

	return
}

func NeedAllBuildModules(bc BuildContext) (modules []Module, err error) {
	moduleAliases, err := NeedAllModuleAliases(bc)
	if err != nil {
		return
	}

	return NeedBuildModules(bc, moduleAliases...)
}

func NeedAllModuleAliases(bc BuildContext) (moduleAliases ModuleAliases, err error) {
	rootModel, err := BuildRootNamespaceModel().Need(bc)
	if err != nil {
		return
	}

	err = ForeachNamespaceModuleAlias(bc, rootModel.GetNamespaceAlias(), func(ma ModuleAlias) error {
		moduleAliases.Append(ma)
		return nil
	})
	return
}

func ForeachNamespaceModuleAlias(bc BuildContext, namespaceAlias NamespaceAlias, each func(ModuleAlias) error) error {
	buildable, err := bc.NeedBuildable(namespaceAlias)
	if err != nil {
		return err
	}
	namespace := buildable.(Namespace)

	for _, moduleAlias := range namespace.GetNamespace().NamespaceModules {
		if err := each(moduleAlias); err != nil {
			return err
		}
	}

	namespaceChildren := namespace.GetNamespace().NamespaceChildren
	if len(namespaceChildren) == 0 {
		return nil
	}

	if err = bc.DependsOn(base.Map(func(na NamespaceAlias) BuildAlias {
		return MakeBuildAlias("Model", na.String())
	}, namespaceChildren...)...); err != nil {
		return err
	}

	for _, namespaceAlias := range namespaceChildren {
		if err := ForeachNamespaceModuleAlias(bc, namespaceAlias, each); err != nil {
			return err
		}
	}

	return nil
}

func GetModuleFromUserInput(bg BuildGraphReadPort, in ModuleAlias) (Module, error) {
	if module, err := FindBuildModule(bg, in); err == nil {
		return module, nil
	}

	if found, err := base.DidYouMean[ModuleAlias](in.String(), bg); err == nil {
		if err = in.Set(found); err != nil {
			return nil, err
		}
		return FindBuildModule(bg, in)
	} else {
		return nil, err
	}
}
