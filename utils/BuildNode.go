package utils

import (
	"fmt"
	"strings"
	"sync"

	"github.com/poppolopoppo/ppb/internal/base"
)

type BuildNode interface {
	BuildAliasable

	GetBuildStamp() BuildStamp
	GetBuildStats() BuildStats
	GetBuildable() Buildable

	DependsOn(...BuildAlias) bool

	GetStaticDependencies() BuildAliases
	GetDynamicDependencies() BuildAliases
	GetOutputDependencies() BuildAliases

	GetDependencyLinks(includeOutputs bool) []BuildDependencyLink
}

type BuildableGeneratedFile interface {
	GetGeneratedFile() Filename
	Buildable
}
type BuildableSourceFile interface {
	GetSourceFile() Filename
	Buildable
}
type BuildableSourceDirectory interface {
	GetSourceDirectory() Directory
	Buildable
}

/***************************************
 * Build Node
 ***************************************/

type buildState struct {
	stats BuildStats
	sync.RWMutex
}

type buildNode struct {
	BuildAlias BuildAlias
	Buildable  Buildable

	Stamp BuildStamp

	Static      BuildDependencies
	Dynamic     BuildDependencies
	OutputFiles BuildDependencies
	OutputNodes BuildAliases

	state  buildState
	future base.AtomicFuture[BuildResult]
}

func newBuildNode(alias BuildAlias, builder Buildable) *buildNode {
	base.Assert(func() bool { return alias.Valid() })
	base.Assert(func() bool { return builder.Alias().Equals(alias) })
	return &buildNode{
		BuildAlias: alias,
		Buildable:  builder,
		Stamp:      BuildStamp{},

		Static:      BuildDependencies{},
		Dynamic:     BuildDependencies{},
		OutputFiles: BuildDependencies{},
		OutputNodes: BuildAliases{},

		state: buildState{
			stats:   BuildStats{},
			RWMutex: sync.RWMutex{},
		},

		future: base.AtomicFuture[BuildResult]{},
	}
}
func (node *buildNode) Alias() BuildAlias { return node.BuildAlias }
func (node *buildNode) String() string    { return node.BuildAlias.String() }

func (node *buildNode) IsMuted() bool {
	node.state.RLock()
	defer node.state.RUnlock()
	switch node.Buildable.(type) {
	case *FileDependency, BuildableSourceFile:
		return true
	case *DirectoryDependency, BuildableSourceDirectory:
		return true
	case BuildableGeneratedFile:
		return false
	default:
		return false
	}
}

func (node *buildNode) GetBuildable() Buildable {
	node.state.RLock()
	defer node.state.RUnlock()
	return node.Buildable
}
func (node *buildNode) GetBuildStamp() BuildStamp {
	node.state.RLock()
	defer node.state.RUnlock()
	return node.Stamp
}
func (node *buildNode) GetBuildStats() BuildStats {
	node.state.RLock()
	defer node.state.RUnlock()
	return node.state.stats
}
func (node *buildNode) GetStaticDependencies() BuildAliases {
	node.state.RLock()
	defer node.state.RUnlock()
	return node.Static.Aliases()
}
func (node *buildNode) GetDynamicDependencies() BuildAliases {
	node.state.RLock()
	defer node.state.RUnlock()
	return node.Dynamic.Aliases()
}
func (node *buildNode) GetOutputDependencies() BuildAliases {
	node.state.RLock()
	defer node.state.RUnlock()
	return append(node.OutputFiles.Aliases(), node.OutputNodes...)
}
func (node *buildNode) GetDependencyLinks(includeOutputs bool) []BuildDependencyLink {
	node.state.RLock()
	defer node.state.RUnlock()
	result := make([]BuildDependencyLink, 0, len(node.Static)+len(node.Dynamic)+len(node.OutputFiles)+len(node.OutputNodes))
	for _, it := range node.Static {
		result = append(result, BuildDependencyLink{Alias: it.Alias, Type: DEPENDENCY_STATIC})
	}
	for _, it := range node.Dynamic {
		result = append(result, BuildDependencyLink{Alias: it.Alias, Type: DEPENDENCY_DYNAMIC})
	}
	if includeOutputs {
		for _, it := range node.OutputFiles {
			result = append(result, BuildDependencyLink{Alias: it.Alias, Type: DEPENDENCY_OUTPUT})
		}
		for _, a := range node.OutputNodes {
			result = append(result, BuildDependencyLink{Alias: a, Type: DEPENDENCY_OUTPUT})
		}
	}
	return result
}
func (node *buildNode) DependsOn(aliases ...BuildAlias) bool {
	node.state.RLock()
	defer node.state.RUnlock()
	for _, a := range aliases {
		if _, ok := node.Static.IndexOf(a); ok {
			return true
		}
	}
	for _, a := range aliases {
		if _, ok := node.Dynamic.IndexOf(a); ok {
			return true
		}
	}
	return false
}
func (node *buildNode) Serialize(ar base.Archive) {
	ar.Serializable(&node.BuildAlias)
	base.SerializeExternal(ar, &node.Buildable)

	ar.Serializable(&node.Stamp)

	ar.Serializable(&node.Static)
	ar.Serializable(&node.Dynamic)
	ar.Serializable(&node.OutputFiles)
	base.SerializeSlice(ar, node.OutputNodes.Ref())
}

func (node *buildNode) makeDirty_AssumeLocked() {
	node.Dynamic = BuildDependencies{}
	node.OutputFiles = BuildDependencies{}
	node.OutputNodes = BuildAliases{}
}

func (node *buildNode) addDynamic_AssumeLocked(a BuildAlias, stamp BuildStamp) {
	base.Assert(func() bool { return stamp.Content.Valid() })
	base.AssertErr(func() error {
		if a.Valid() {
			return nil
		}
		return fmt.Errorf("%v: invalid empty alias for dynamic dependency", node)
	})
	base.AssertErr(func() error {
		if !node.Alias().Equals(a) {
			return nil
		}
		return fmt.Errorf("%v: can't have an output dependency to self", a)
	})
	base.AssertErr(func() error {
		if _, ok := node.Static.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: dynamic dependency is already static <%v>", node, a)
	})
	base.AssertErr(func() error {
		if _, ok := node.OutputFiles.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: dynamic dependency is already output <%v>", node, a)
	})
	base.AssertErr(func() error {
		if ok := node.OutputNodes.Contains(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: dynamic dependency is already output <%v>", node, a)
	})

	node.Dynamic.Add(a, stamp)
}
func (node *buildNode) addOutputFile_AssumeLocked(a BuildAlias, stamp BuildStamp) {
	base.Assert(func() bool { return stamp.Content.Valid() })
	base.AssertErr(func() error {
		if a.Valid() {
			return nil
		}
		return fmt.Errorf("%v: invalid empty alias for output file dependency", node)
	})
	base.AssertErr(func() error {
		if !node.Alias().Equals(a) {
			return nil
		}
		return fmt.Errorf("%v: can't have an output dependency to self", a)
	})
	base.AssertErr(func() error {
		if _, ok := node.Static.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: output file dependency is already static <%v>", node, a)
	})
	base.AssertErr(func() error {
		if _, ok := node.Dynamic.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: output file dependency is already dynamic <%v>", node, a)
	})
	base.AssertErr(func() error {
		if ok := node.OutputNodes.Contains(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: output file dependency is already output node <%v>", node, a)
	})

	node.OutputFiles.Add(a, stamp)
}
func (node *buildNode) addOutputNode_AssumeLocked(a BuildAlias) {
	base.AssertErr(func() error {
		if a.Valid() {
			return nil
		}
		return fmt.Errorf("%v: invalid empty alias for output dependency", node)
	})
	base.AssertErr(func() error {
		if !node.Alias().Equals(a) {
			return nil
		}
		return fmt.Errorf("%v: can't have an output dependency to self", a)
	})
	base.AssertErr(func() error {
		if _, ok := node.Static.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: output node is already static dependency <%v>", node, a)
	})
	base.AssertErr(func() error {
		if _, ok := node.Dynamic.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: output node is already dynamic dependency <%v>", node, a)
	})
	base.AssertErr(func() error {
		if _, ok := node.OutputFiles.IndexOf(a); !ok {
			return nil
		}
		return fmt.Errorf("%v: output node dependency is already output file dependency <%v>", node, a)
	})

	node.OutputNodes.AppendUniq(a)
}

/***************************************
 * Build Dependency Type
 ***************************************/

type BuildDependencyType int32

const (
	DEPENDENCY_ROOT   BuildDependencyType = -1
	DEPENDENCY_OUTPUT BuildDependencyType = iota
	DEPENDENCY_STATIC
	DEPENDENCY_DYNAMIC
)

func (x *BuildDependencyType) Combine(y BuildDependencyType) {
	if *x > y || *x == DEPENDENCY_ROOT {
		*x = y
	}
}

func (x BuildDependencyType) String() string {
	switch x {
	case DEPENDENCY_STATIC:
		return "STATIC"
	case DEPENDENCY_DYNAMIC:
		return "DYNAMIC"
	case DEPENDENCY_OUTPUT:
		return "OUTPUT"
	case DEPENDENCY_ROOT:
		return "ROOT"
	default:
		base.UnexpectedValuePanic(x, x)
		return ""
	}
}
func (x *BuildDependencyType) Set(in string) error {
	switch strings.ToUpper(in) {
	case DEPENDENCY_STATIC.String():
		*x = DEPENDENCY_STATIC
	case DEPENDENCY_DYNAMIC.String():
		*x = DEPENDENCY_DYNAMIC
	case DEPENDENCY_OUTPUT.String():
		*x = DEPENDENCY_OUTPUT
	case DEPENDENCY_ROOT.String():
		*x = DEPENDENCY_ROOT
	default:
		return base.MakeUnexpectedValueError(x, in)
	}
	return nil
}

/***************************************
 * Build Dependencies
 ***************************************/

type BuildDependency struct {
	Alias BuildAlias
	Stamp BuildStamp
}

type BuildDependencies []BuildDependency

func (x *BuildDependency) Serialize(ar base.Archive) {
	ar.Serializable(&x.Alias)
	ar.Serializable(&x.Stamp)
}

type BuildDependencyLink struct {
	Alias BuildAlias
	Type  BuildDependencyType
}

func (deps BuildDependencies) Aliases() BuildAliases {
	result := make(BuildAliases, len(deps))
	for i, it := range deps {
		result[i] = it.Alias
	}
	return result
}
func (deps BuildDependencies) Copy() BuildDependencies {
	return base.CopySlice(deps...)
}
func (deps *BuildDependencies) Add(alias BuildAlias, stamp BuildStamp) {
	if i, ok := deps.IndexOf(alias); ok {
		(*deps)[i].Stamp = stamp
	} else {
		*deps = append(*deps, BuildDependency{Alias: alias, Stamp: stamp})
	}
}
func (deps BuildDependencies) IndexOf(alias BuildAlias) (int, bool) {
	// should not be used in critical path... or sort the slice and use binary search
	for i, it := range deps {
		if it.Alias == alias {
			return i, true
		}
	}
	return len(deps), false
}
func (deps *BuildDependencies) Serialize(ar base.Archive) {
	base.SerializeSlice(ar, (*[]BuildDependency)(deps))
}
func (deps BuildDependencies) validate(owner BuildNode, depType BuildDependencyType) bool {
	valid := true
	for _, it := range deps {
		if !it.Stamp.Content.Valid() {
			valid = false
			base.LogError(LogBuildGraph, "%v: %s dependency <%v> has an invalid build stamp (%v)", depType, owner.Alias(), it.Alias, it.Stamp)
		}
	}
	return valid
}
func (deps *BuildDependencies) makeDirty() {
	for i := range *deps {
		(*deps)[i].Stamp = BuildStamp{}
	}
}
func (deps *BuildDependencies) updateBuild(owner BuildNode, depType BuildDependencyType, results []BuildResult) (rebuild bool) {
	base.Assert(func() bool { return len(results) == len(*deps) })

	for _, result := range results {
		alias := result.Alias()
		base.Assert(func() bool { return result.Content.Valid() })

		oldStampIndex, ok := deps.IndexOf(alias)
		base.AssertIn(ok, true)
		oldStamp := (*deps)[oldStampIndex].Stamp

		if oldStamp != result.BuildStamp {
			base.LogTrace(LogBuildGraph, "%v: %v dependency <%v> has been updated:\n\tnew: %v\n\told: %v", owner.Alias(), depType, alias, result.BuildStamp, oldStamp)

			deps.Add(alias, result.BuildStamp)
			rebuild = true
		} // else // LogDebug("%v: %v %v dependency is up-to-date", owner.Alias(), alias, depType)
	}

	return rebuild
}
