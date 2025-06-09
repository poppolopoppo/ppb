package compile

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/poppolopoppo/ppb/internal/base"
	"github.com/poppolopoppo/ppb/utils"

	internal_io "github.com/poppolopoppo/ppb/internal/io"
)

var LogModel = base.NewLogCategory("Model")

const PCH_DEFAULT_HEADER = "stdafx.h"
const PCH_DEFAULT_SOURCE = "stdafx.cpp"

func GetRootNamespaceName() string {
	return utils.CommandEnv.Prefix()
}

/***************************************
 * ModelImporter
 ***************************************/

type ModelRecursiveDependencyError struct {
	A, B ModuleAlias
}

func (x ModelRecursiveDependencyError) Error() string {
	return fmt.Sprintf("found recursive dependency between %q and %q, modules can't depend from each other", x.A, x.B)
}

type ModelMissingModuleError struct {
	Alias ModuleAlias
}

func (x ModelMissingModuleError) Error() string {
	return fmt.Sprintf("can't find module %q, did you miss some declaration?", x.Alias)
}

type ModelImporter struct {
	Source     utils.Directory
	Import     utils.FileSet
	Modules    ModuleAliases
	Namespaces NamespaceAliases
}

func (x *ModelImporter) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("ModelImporter", x.Source.Path)
}
func (x *ModelImporter) Build(bc utils.BuildContext) (err error) {
	x.Import, err = internal_io.GlobDirectory(bc, x.Source, base.NewStringSet("*"+MODULEMODEL_EXT), base.StringSet{}, utils.FileSet{})
	if err != nil {
		return
	}

	x.Modules = make(ModuleAliases, 0, len(x.Import))
	x.Namespaces = make(NamespaceAliases, 0, len(x.Import))

	namespaces := make(map[utils.Directory]*NamespaceRules, len(x.Import))
	getOrCreateNamespaceRules := func(namespaceDir utils.Directory) (namespace *NamespaceRules) {
		var ok bool
		if namespace, ok = namespaces[namespaceDir]; !ok {
			namespace = &NamespaceRules{
				NamespaceAlias: NamespaceAlias{namespaceDir.Relative(x.Source)},
				NamespaceDir:   namespaceDir,
			}

			if len(namespace.NamespaceAlias.NamespaceName) == 0 || namespace.NamespaceAlias.NamespaceName == "." {
				namespace.NamespaceAlias.NamespaceName = GetRootNamespaceName()
			}

			x.Namespaces.Append(namespace.NamespaceAlias)
			namespaces[namespaceDir] = namespace
		}
		return
	}

	futureModules := base.Map(func(moduleFile utils.Filename) base.Future[*ModuleModel] {
		namespaceDir := moduleFile.Dirname.Parent()
		namespace := getOrCreateNamespaceRules(namespaceDir)

		// register the namespace in its parent recursively
		for {
			parentNamespaceDir := namespaceDir.Parent()
			parentNamespace := getOrCreateNamespaceRules(parentNamespaceDir)
			parentNamespace.NamespaceChildren.AppendUniq(namespace.NamespaceAlias)

			if parentNamespaceDir.Equals(x.Source) {
				break // reached the root namespace
			}
		}

		return BuildModuleModel(moduleFile, namespaceDir.Relative(x.Source)).Prepare(bc)
	}, x.Import...)

	checkModuleDeps := make(map[ModuleAlias]*ModuleModel, len(x.Import))
	err = base.ParallelJoin(func(_ int, module *ModuleModel) error {
		moduleAlias := module.GetModuleAlias()

		if module.hasAllowedPlatforms(moduleAlias) {
			namespace := getOrCreateNamespaceRules(module.Source.Dirname.Parent())
			namespace.NamespaceModules.Append(moduleAlias)
			x.Modules.Append(moduleAlias)

			if m, err := bc.NeedBuildable(module); err == nil {
				base.AssertIn(utils.Buildable(module), m)
				checkModuleDeps[moduleAlias] = module
			} else {
				return err
			}
		}

		return nil
	}, futureModules...)
	if err != nil {
		return
	}

	// validate module dependencies
	inspectModuleDeps := func(ma ModuleAlias, deps ...ModuleAlias) error {
		for _, it := range deps {
			if other, ok := checkModuleDeps[it]; !ok {
				return ModelMissingModuleError{it}
			} else if ma.Compare(it) <= 0 && other.hasModuleDependency(ma) {
				return ModelRecursiveDependencyError{A: ma, B: it}
			}
		}
		return nil
	}
	for ma, module := range checkModuleDeps {
		if err = inspectModuleDeps(ma, module.PrivateDependencies...); err != nil {
			return err
		}
		if err = inspectModuleDeps(ma, module.PublicDependencies...); err != nil {
			return err
		}
		if err = inspectModuleDeps(ma, module.RuntimeDependencies...); err != nil {
			return err
		}
	}

	// sort the modules and namespaces by their names for determinism
	sort.Slice(x.Modules, func(i, j int) bool {
		return x.Modules[i].Compare(x.Modules[j]) < 0
	})
	sort.Slice(x.Namespaces, func(i, j int) bool {
		return x.Namespaces[i].Compare(x.Namespaces[j]) < 0
	})

	// output the namespace rules for each namespace
	for _, namespace := range namespaces {
		if _, err = utils.WrapBuildFactory(func(bi utils.BuildInitializer) (*NamespaceRules, error) {
			return namespace, nil
		}).Output(bc); err != nil {
			return
		}
	}
	return
}
func (x *ModelImporter) Serialize(ar base.Archive) {
	ar.Serializable(&x.Source)
	ar.Serializable(&x.Import)
	base.SerializeSlice(ar, x.Modules.Ref())
	base.SerializeSlice(ar, x.Namespaces.Ref())
}

func BuildModelImporter(source utils.Directory) utils.BuildFactoryTyped[*ModelImporter] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (ModelImporter, error) {
		if err := bi.NeedDirectories(source); err != nil {
			return ModelImporter{}, err
		}
		return ModelImporter{
			Source: source.Normalize(),
		}, nil
	})
}

/***************************************
 * Module Model
 ***************************************/

const MODULEMODEL_EXT = "-module.json"

type ModuleModel struct {
	// following fields are deduced and not serialized
	Name      string         `json:"-"`
	Source    utils.Filename `json:"-"`
	Namespace string         `json:"-"`

	ModuleType ModuleType `json:",omitempty" jsonschema:"description=Type of module, such as library, executable, etc."`

	SourceDirs    base.StringSet `json:",omitempty" jsonschema:"description=Directories to search for source files, relative to the module directory"`
	SourceGlobs   base.StringSet `json:",omitempty" jsonschema:"description=Glob patterns to match source files, relative to the module directory"`
	ExcludedGlobs base.StringSet `json:",omitempty" jsonschema:"description=Glob patterns to exclude from source file matching, relative to the module directory"`
	SourceFiles   base.StringSet `json:",omitempty" jsonschema:"description=List of source files, relative to the module directory"`
	ExcludedFiles base.StringSet `json:",omitempty" jsonschema:"description=List of files to exclude from the module, relative to the module directory"`
	ForceIncludes base.StringSet `json:",omitempty" jsonschema:"description=List of files to include in the module, even if excluded"`
	IsolatedFiles base.StringSet `json:",omitempty" jsonschema:"description=List of files that are isolated from the module unity build, relative to the module directory"`
	ExtraFiles    base.StringSet `json:",omitempty" jsonschema:"description=List of extra files to include in the module, in addition to the source files, relative to the module directory"`
	ExtraDirs     base.StringSet `json:",omitempty" jsonschema:"description=List of extra directories to include in the module, in addition to the source directories, relative to the module directory"`

	PrecompiledHeader utils.StringVar `json:",omitempty" jsonschema:"description=Precompiled header file for the module, relative to the module directory"`
	PrecompiledSource utils.StringVar `json:",omitempty" jsonschema:"description=Precompiled source file for the module, relative to the module directory"`

	PrivateDependencies ModuleAliases `json:",omitempty" jsonschema:"description=List of private dependencies for the module, which are not exposed to other modules"`
	PublicDependencies  ModuleAliases `json:",omitempty" jsonschema:"description=List of public dependencies for the module, which are exposed to other modules"`
	RuntimeDependencies ModuleAliases `json:",omitempty" jsonschema:"description=List of runtime dependencies for the module, which are required at runtime"`

	Archetypes       ModuleArchetypeAliases      `json:",omitempty" jsonschema:"description=Archetypes of this module, which are applied to the module rules"`
	AllowedPlatforms base.SetT[PlatformAlias]    `json:",omitempty" jsonschema:"description=List of allowed platforms for this module, which are applied to the module rules"`
	HAL              map[base.HostId]ModuleModel `json:",omitempty" jsonschema:"description=Platform-specific module extensions, which are applied to the module rules"`
	TAG              map[TagFlags]ModuleModel    `json:",omitempty" jsonschema:"description=Tag-specific module extensions, which are applied to the module rules"`

	Facet
	CppRules
}

func BuildModuleModel(source utils.Filename, namespace string) utils.BuildFactoryTyped[*ModuleModel] {
	return utils.MakeBuildFactory(func(bi utils.BuildInitializer) (ModuleModel, error) {
		source = source.Normalize()
		name := strings.TrimSuffix(source.Basename, MODULEMODEL_EXT)

		return ModuleModel{
			Name:      name,
			Source:    source,
			Namespace: namespace,
		}, bi.NeedFiles(source)
	})
}

func (x *ModuleModel) Alias() utils.BuildAlias {
	return utils.MakeBuildAlias("Model", x.Namespace, x.Name)
}
func (x *ModuleModel) GetModuleAlias() ModuleAlias {
	return ModuleAlias{
		NamespaceAlias: NamespaceAlias{x.Namespace},
		ModuleName:     x.Name,
	}
}
func (x *ModuleModel) Build(bc utils.BuildContext) error {
	*x = ModuleModel{
		Name:      x.Name,
		Source:    x.Source,
		Namespace: x.Namespace,
	}

	if err := utils.UFS.OpenBuffered(x.Source, func(r io.Reader) error {
		return base.JsonDeserialize(x, r)
	}); err != nil {
		return err
	}

	moduleAlias := x.GetModuleAlias()
	moduleDir := x.Source.Dirname

	if !x.hasAllowedPlatforms(moduleAlias) {
		return nil
	}

	rules, err := x.createModuleRules(moduleAlias)
	if err != nil {
		return err
	}

	if err := x.applyArchetypes(&rules, moduleAlias.Alias()); err != nil {
		return err
	}

	rules.ForceIncludes.Append(utils.MakeFileSet(moduleDir, x.ForceIncludes...)...)
	rules.Source.ExtraFiles.AppendUniq(x.Source)

	if !x.PrecompiledHeader.IsInheritable() {
		f := moduleDir.AbsoluteFile(x.PrecompiledHeader.Get()).Normalize()
		rules.PrecompiledHeader = f
	} else if f := moduleDir.File(PCH_DEFAULT_HEADER); f.Exists() {
		rules.PrecompiledHeader = f
	}
	if !x.PrecompiledSource.IsInheritable() {
		f := moduleDir.AbsoluteFile(x.PrecompiledSource.Get()).Normalize()
		rules.PrecompiledSource = f
	} else if f := moduleDir.File(PCH_DEFAULT_SOURCE); f.Exists() {
		rules.PrecompiledSource = f
	}

	_, err = bc.OutputFactory(utils.WrapBuildFactory(func(bi utils.BuildInitializer) (*ModuleRules, error) {
		dependencyAliases := make(utils.BuildAliases, 0, len(x.PrivateDependencies)+len(x.PublicDependencies)+len(x.RuntimeDependencies))

		for _, moduleAlias := range x.PrivateDependencies {
			dependencyAliases.Append(utils.MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}

		for _, moduleAlias := range x.PublicDependencies {
			dependencyAliases.Append(utils.MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}

		for _, moduleAlias := range x.RuntimeDependencies {
			dependencyAliases.Append(utils.MakeBuildAlias("Model", moduleAlias.NamespaceName, moduleAlias.ModuleName))
		}

		return &rules, bi.DependsOn(dependencyAliases...)
	}), utils.OptionBuildForce)
	return err
}
func (x *ModuleModel) createModuleRules(moduleAlias ModuleAlias) (ModuleRules, error) {
	x.applyHAL(x, moduleAlias.Alias())

	moduleDir := x.Source.Dirname
	rules := ModuleRules{
		ModuleAlias: moduleAlias,
		ModuleDir:   moduleDir,
		ModuleType:  x.ModuleType,
		CppRules:    x.CppRules,
		Source: ModuleSource{
			SourceGlobs:   x.SourceGlobs,
			ExcludedGlobs: x.ExcludedGlobs,
			SourceDirs:    utils.MakeDirSet(moduleDir, x.SourceDirs...).Normalize(),
			SourceFiles:   utils.MakeFileSet(moduleDir, x.SourceFiles...).Normalize(),
			ExcludedFiles: utils.MakeFileSet(moduleDir, x.ExcludedFiles...).Normalize(),
			IsolatedFiles: utils.MakeFileSet(moduleDir, x.IsolatedFiles...).Normalize(),
			ExtraFiles:    utils.MakeFileSet(moduleDir, x.ExtraFiles...).Normalize(),
			ExtraDirs:     utils.MakeDirSet(moduleDir, x.ExtraDirs...).Normalize(),
		},
		PrivateDependencies: x.PrivateDependencies,
		PublicDependencies:  x.PublicDependencies,
		RuntimeDependencies: x.RuntimeDependencies,
		Facet:               x.Facet,
		PerTags:             map[TagFlags]ModuleRules{},
	}

	for tags, model := range x.TAG {
		if model.hasAllowedPlatforms(moduleAlias) {
			var err error
			rules.PerTags[tags], err = model.createModuleRules(moduleAlias)
			if err != nil {
				return ModuleRules{}, err
			}
		}
	}

	return rules, nil
}
func (x *ModuleModel) Serialize(ar base.Archive) {
	ar.String(&x.Name)
	ar.Serializable(&x.Source)
	ar.String(&x.Namespace)

	ar.Serializable(&x.ModuleType)

	ar.Serializable(&x.SourceDirs)
	ar.Serializable(&x.SourceGlobs)
	ar.Serializable(&x.ExcludedGlobs)
	ar.Serializable(&x.SourceFiles)
	ar.Serializable(&x.ExcludedFiles)
	ar.Serializable(&x.ForceIncludes)
	ar.Serializable(&x.IsolatedFiles)
	ar.Serializable(&x.ExtraFiles)
	ar.Serializable(&x.ExtraDirs)

	ar.Serializable(&x.PrecompiledHeader)
	ar.Serializable(&x.PrecompiledSource)

	base.SerializeSlice(ar, x.PrivateDependencies.Ref())
	base.SerializeSlice(ar, x.PublicDependencies.Ref())
	base.SerializeSlice(ar, x.RuntimeDependencies.Ref())

	ar.Serializable(&x.CppRules)

	base.SerializeSlice(ar, x.Archetypes.Ref())
	base.SerializeSlice(ar, x.AllowedPlatforms.Ref())
	base.SerializeMap(ar, &x.HAL)
	base.SerializeMap(ar, &x.TAG)
	ar.Serializable(&x.Facet)
}
func (x *ModuleModel) Append(o *ModuleModel) {
	base.Inherit(&x.ModuleType, o.ModuleType)

	x.SourceDirs.Append(o.SourceDirs...)
	x.SourceGlobs.Append(o.SourceGlobs...)
	x.ExcludedGlobs.Append(o.ExcludedGlobs...)
	x.SourceFiles.Append(o.SourceFiles...)
	x.ExcludedFiles.Append(o.ExcludedFiles...)
	x.ForceIncludes.Append(o.ForceIncludes...)
	x.IsolatedFiles.Append(o.IsolatedFiles...)
	x.ExtraFiles.Append(o.ExtraFiles...)
	x.ExtraDirs.Append(o.ExtraDirs...)

	x.PrecompiledHeader.Inherit(o.PrecompiledHeader)
	x.PrecompiledSource.Inherit(o.PrecompiledSource)

	x.PrivateDependencies.Append(o.PrivateDependencies...)
	x.PublicDependencies.Append(o.PublicDependencies...)
	x.RuntimeDependencies.Append(o.RuntimeDependencies...)

	x.CppRules.Inherit(&o.CppRules)

	x.Archetypes.AppendUniq(o.Archetypes...)
	x.AllowedPlatforms.AppendUniq(o.AllowedPlatforms...)

	for k, v := range o.HAL {
		if w, ok := x.HAL[k]; ok {
			w.Append(&v)
			x.HAL[k] = w
		} else {
			x.HAL[k] = v
		}
	}
	for k, v := range o.TAG {
		if w, ok := x.TAG[k]; ok {
			w.Append(&v)
			x.TAG[k] = w
		} else {
			x.TAG[k] = v
		}
	}

	x.Facet.Append(&o.Facet)
}
func (x *ModuleModel) Prepend(o *ModuleModel) {
	base.Overwrite(&x.ModuleType, o.ModuleType)

	x.SourceDirs.Prepend(o.SourceDirs...)
	x.SourceGlobs.Prepend(o.SourceGlobs...)
	x.ExcludedGlobs.Prepend(o.ExcludedGlobs...)
	x.SourceFiles.Prepend(o.SourceFiles...)
	x.ExcludedFiles.Prepend(o.ExcludedFiles...)
	x.ForceIncludes.Prepend(o.ForceIncludes...)
	x.IsolatedFiles.Prepend(o.IsolatedFiles...)
	x.ExtraFiles.Prepend(o.ExtraFiles...)
	x.ExtraDirs.Prepend(o.ExtraDirs...)

	x.PrecompiledHeader.Overwrite(o.PrecompiledHeader)
	x.PrecompiledSource.Overwrite(o.PrecompiledSource)

	x.PrivateDependencies.Prepend(o.PrivateDependencies...)
	x.PublicDependencies.Prepend(o.PublicDependencies...)
	x.RuntimeDependencies.Prepend(o.RuntimeDependencies...)

	x.CppRules.Inherit(&o.CppRules)

	x.Archetypes.PrependUniq(o.Archetypes...)
	x.AllowedPlatforms.PrependUniq(o.AllowedPlatforms...)

	for k, v := range o.HAL {
		if w, ok := x.HAL[k]; ok {
			w.Prepend(&v)
			x.HAL[k] = w
		} else {
			x.HAL[k] = v
		}
	}
	for k, v := range o.TAG {
		if w, ok := x.TAG[k]; ok {
			w.Prepend(&v)
			x.TAG[k] = w
		} else {
			x.TAG[k] = v
		}
	}

	x.Facet.Prepend(&o.Facet)
}

func (src *ModuleModel) hasModuleDependency(it ...ModuleAlias) bool {
	return src.PrivateDependencies.Contains(it...) ||
		src.PublicDependencies.Contains(it...) ||
		src.RuntimeDependencies.Contains(it...)
}

func (src *ModuleModel) hasAllowedPlatforms(name fmt.Stringer) bool {
	if len(src.AllowedPlatforms) > 0 {
		localPlatform := GetLocalHostPlatformAlias()
		if !src.AllowedPlatforms.Contains(localPlatform) {
			base.LogTrace(LogModel, "%v: not allowed on <%v> platform", name, localPlatform)
			return false
		}
	}
	return true
}
func (src *ModuleModel) applyArchetypes(rules *ModuleRules, name utils.BuildAlias) error {
	return src.Archetypes.Range(func(id ModuleArchetypeAlias) error {
		if decorator, ok := AllArchetypes.Get(id.String()); ok {
			base.LogTrace(LogModel, "%v: inherit module archtype <%v>", name, id)
			if err := decorator(rules); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%v: invalid module archtype <%v>", name, id)
		}
		return nil
	})
}
func (src *ModuleModel) applyHAL(model *ModuleModel, name utils.BuildAlias) {
	hostId := base.GetCurrentHost().Id
	for id, other := range src.HAL {
		var hal base.HostId
		if err := hal.Set(id.String()); err == nil && hal == hostId {
			base.LogTrace(LogModel, "%v: inherit platform facet [%v]", name, id)
			model.Prepend(&other)
		} else if err != nil {
			base.LogError(LogModel, "%v: invalid platform id [%v], %v", name, id, err)
		}
	}
}
