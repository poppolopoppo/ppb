package utils

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

const BUILDGRAPH_ENABLE_CHECKS = false // %_NOCOMMIT%

var LogBuildGraph = base.NewLogCategory("BuildGraph")
var LogBuildEvent = base.NewLogCategory("BuildEvent")

/***************************************
 * Public API
 ***************************************/

type Buildable interface {
	BuildAliasable
	base.Serializable
	Build(BuildContext) error
}

type BuildStamp struct {
	ModTime time.Time
	Content base.Fingerprint
}

type BuildResult struct {
	Buildable
	BuildStamp
}

type BuildNodeEvent struct {
	Alias     BuildAlias
	Node      BuildNode
	Buildable Buildable
}

type BuildGraph interface {
	Aliases() []BuildAlias
	Dirty() bool

	GlobalContext(options ...BuildOptionFunc) BuildContext

	Find(alias BuildAlias) BuildNode
	Create(buildable Buildable, staticDeps BuildAliases, options ...BuildOptionFunc) BuildNode
	Build(alias BuildAliasable, options ...BuildOptionFunc) (BuildNode, base.Future[BuildResult])
	BuildMany(aliases BuildAliases, options ...BuildOptionFunc) base.Future[[]BuildResult]
	Join() error

	Load(io.Reader) error
	PostLoad()
	Save(io.Writer) error

	GetStaticDependencies(BuildNode) []BuildNode
	GetDynamicDependencies(BuildNode) []BuildNode
	GetOutputDependencies(BuildNode) []BuildNode

	GetDependencyChain(a, b BuildAlias) ([]BuildDependencyLink, error)
	GetDependencyInputFiles(BuildAlias) (FileSet, error)
	GetDependencyLinks(BuildAlias) ([]BuildDependencyLink, error)

	GetBuildStats() BuildStats
	GetMostExpansiveNodes(n int, inclusive bool) []BuildNode

	OnBuildGraphStart(base.EventDelegate[BuildGraph]) base.DelegateHandle
	OnBuildNodeStart(base.EventDelegate[BuildNodeEvent]) base.DelegateHandle
	OnBuildNodeFinished(base.EventDelegate[BuildNodeEvent]) base.DelegateHandle
	OnBuildGraphFinished(base.EventDelegate[BuildGraph]) base.DelegateHandle

	RemoveOnBuildGraphStart(base.DelegateHandle) bool
	RemoveOnBuildNodeStart(base.DelegateHandle) bool
	RemoveOnBuildNodeFinished(base.DelegateHandle) bool
	RemoveOnBuildGraphFinished(base.DelegateHandle) bool

	base.Equatable[BuildGraph]
	base.Serializable
}

/***************************************
 * Build Graph
 ***************************************/

type buildGraph struct {
	flags   *CommandFlags
	nodes   *base.SharedStringMapT[*buildNode]
	options BuildOptions
	stats   BuildStats

	revision int32

	buildEvents
}

func NewBuildGraph(flags *CommandFlags, options ...BuildOptionFunc) BuildGraph {
	result := &buildGraph{
		flags:       flags,
		nodes:       base.NewSharedStringMap[*buildNode](1000),
		options:     NewBuildOptions(options...),
		revision:    0,
		buildEvents: newBuildEvents(),
	}
	return result
}

func (g *buildGraph) Aliases() []BuildAlias {
	keys := g.nodes.Keys()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return base.Map(func(in string) BuildAlias { return BuildAlias(in) }, keys...)
}
func (g *buildGraph) Dirty() bool {
	return atomic.LoadInt32(&g.revision) > 0
}
func (g *buildGraph) GlobalContext(options ...BuildOptionFunc) BuildContext {
	bo := NewBuildOptions(options...)
	return &buildGraphContext{graph: g, options: &bo}
}
func (g *buildGraph) Find(alias BuildAlias) (result BuildNode) {
	if node, _ := g.nodes.Get(alias.String()); node != nil {
		base.Assert(func() bool { return node.Alias().Equals(alias) })
		base.AssertErr(func() error {
			if node.Buildable.Alias().Equals(alias) {
				return nil
			}
			return fmt.Errorf("%v: node alias do not match buildable -> %q",
				alias, base.MakeStringer(func() string {
					return node.Buildable.Alias().String()
				}))
		})

		base.LogTrace(LogBuildGraph, "Find(%q) -> %T", alias, node.Buildable)
		result = node
	} else {
		base.LogTrace(LogBuildGraph, "Find(%q) -> NOT_FOUND", alias)
	}

	return
}

func (g *buildGraph) Create(buildable Buildable, static BuildAliases, options ...BuildOptionFunc) BuildNode {
	bo := NewBuildOptions(options...)

	var node *buildNode
	var loaded bool

	alias := buildable.Alias()
	base.AssertErr(func() error {
		if alias.Valid() {
			return nil
		}
		return fmt.Errorf("invalid alias for <%T>", buildable)
	})
	base.LogTrace(LogBuildGraph, "create <%T> node %q (force: %v, dirty: %v)", buildable, alias, bo.Force, bo.Dirty)

	if node, loaded = g.nodes.Get(alias.String()); loaded {
		// quick reject if a node with same alias already exists
		if !(bo.Force || bo.Dirty) {
			return node
		}
	} else {
		base.Assert(func() bool {
			makeCaseInsensitive := func(in string) string {
				return SanitizePath(strings.ToLower(in), '/')
			}
			lowerAlias := makeCaseInsensitive(alias.String())
			for _, key := range g.nodes.Keys() {
				lowerKey := makeCaseInsensitive(key)
				if lowerKey == lowerAlias {
					if key != alias.String() {
						base.LogError(LogBuildGraph, "alias already registered with different case:\n\t add: %v\n\tfound: %v", alias, key)
						return false
					}
					break
				}
			}
			return true
		})

		// first optimistic Get() to avoid newBuildNode() bellow
		node, loaded = g.nodes.FindOrAdd(alias.String(), newBuildNode(alias, buildable))
	}

	defer base.LogPanicIfFailed(LogBuildGraph, bo.OnBuilt.Invoke(node))

	node.state.Lock()
	defer node.state.Unlock()

	base.Assert(func() bool { return alias == node.Alias() })
	base.AssertSameType(node.Buildable, buildable)

	base.LogPanicIfFailed(LogBuildGraph, bo.OnLaunched.Invoke(node))

	dirty := !loaded || len(static) != len(node.Static)
	newStaticDeps := make(BuildDependencies, 0, len(static))

	for _, a := range static {
		var oldStamp BuildStamp
		if old, hit := node.Static.IndexOf(a); hit {
			oldStamp = node.Static[old].Stamp
		} else {
			dirty = true
		}
		newStaticDeps = append(newStaticDeps, BuildDependency{
			Alias: a,
			Stamp: oldStamp,
		})
	}

	if !dirty {
		if bo.Dirty {
			dirty = true
		} else if bo.Force { // compare content of buildable objects
			dirty = MakeBuildFingerprint(buildable) != MakeBuildFingerprint(node.Buildable)
		}
	}

	node.Buildable = buildable
	node.Static = newStaticDeps

	if dirty {
		base.LogDebug(LogBuildGraph, "%v: dirty <%v> node depending on %v%v", alias,
			base.MakeStringer(func() string { return reflect.TypeOf(node.Buildable).String() }),
			node.Static.Aliases(),
			base.Blend("", " (forced update)", bo.Force))

		node.makeDirty_AssumeLocked()
		node.Static.makeDirty()

		g.makeDirty()
	}

	// node just went through a reset -> forget cached future if any
	node.future.Reset()

	base.AssertErr(func() error {
		if node.Buildable.Alias().Equals(alias) {
			return nil
		}
		return fmt.Errorf("%v: node alias do not match buildable -> %q",
			alias, base.MakeStringer(func() string {
				return node.Buildable.Alias().String()
			}))
	})
	return node
}

func (g *buildGraph) Build(it BuildAliasable, options ...BuildOptionFunc) (BuildNode, base.Future[BuildResult]) {
	a := it.Alias()
	base.AssertErr(func() error {
		if a.Valid() {
			return nil
		}
		return fmt.Errorf("invalid alias for <%T>", it)
	})

	if node, ok := g.nodes.Get(a.String()); ok {
		base.Assert(func() bool { return node.Alias().Equals(a) })
		base.AssertErr(func() error {
			if node.Buildable.Alias().Equals(a) {
				return nil
			}
			return fmt.Errorf("%v: node alias do not match buildable -> %q",
				a, base.MakeStringer(func() string {
					return node.Buildable.Alias().String()
				}))
		})

		bo := NewBuildOptions(options...)
		return node, g.launchBuild(node, &bo)
	} else {
		return nil, base.MakeFutureError[BuildResult](fmt.Errorf("build: unknown node %q", a))
	}
}

func (g *buildGraph) BuildMany(targets BuildAliases, options ...BuildOptionFunc) (result base.Future[[]BuildResult]) {
	switch len(targets) {
	case 0:
		return base.MakeFutureLiteral([]BuildResult{})
	case 1:
		alias := targets[0]
		_, future := g.Build(alias, options...)

		return base.MapFuture(future, func(it BuildResult) ([]BuildResult, error) {
			return []BuildResult{it}, nil
		})
	default:
		return base.MakeFuture(func() (results []BuildResult, err error) {
			results = make([]BuildResult, len(targets))

			bo := NewBuildOptions(options...)
			boStruct := OptionBuildStruct(&bo)

			err = base.ParallelJoin(
				func(i int, it BuildResult) error {
					base.Assert(func() bool { return it.Content.Valid() })
					results[i] = it
					return nil
				},
				base.Map(func(alias BuildAlias) base.Future[BuildResult] {
					_, future := g.Build(alias, boStruct)
					return future
				}, targets...)...)

			return
		})
	}
}

func (g *buildGraph) Join() (lastErr error) {
	for lastErr == nil && g.hasRunningTasks() {
		lastErr = g.nodes.Range(func(_ string, node *buildNode) error {
			if node != nil {
				if future := node.future.Load(); future != nil {
					return future.Join().Failure()
				}
			}
			return nil
		})
	}
	return
}

func (g *buildGraph) PostLoad() {
	if g.flags.Purge.Get() {
		g.revision = 0
		g.nodes.Clear()
		g.makeDirty()
	}
}
func (g *buildGraph) Serialize(ar base.Archive) {
	var pinned []*buildNode
	serialize := func(node **buildNode) {
		*node = new(buildNode)
		ar.Serializable(*node)
	}
	if !ar.Flags().IsLoading() {
		serialize = func(node **buildNode) {
			ar.Serializable(*node)
		}
		pinned = g.nodes.Values()
		sort.Slice(pinned, func(i, j int) bool {
			return pinned[i].BuildAlias.Compare(pinned[j].BuildAlias) < 0
		})
	}
	base.SerializeMany(ar, serialize, &pinned)
	if ar.Flags().IsLoading() && ar.Error() == nil {
		g.nodes.Clear()
		for _, node := range pinned {
			g.nodes.Add(node.Alias().String(), node)
		}
	}
}
func (g *buildGraph) Save(dst io.Writer) error {
	g.revision = 0
	return base.CompressedArchiveFileWrite(dst, g.Serialize)
}
func (g *buildGraph) Load(src io.Reader) error {
	g.revision = 0
	file, err := base.CompressedArchiveFileRead(src, g.Serialize)
	base.LogVeryVerbose(LogBuildGraph, "archive version = %v tags = %v", file.Version, file.Tags)
	return err
}
func (g *buildGraph) Equals(other BuildGraph) bool {
	return other.(*buildGraph) == g
}

func (g *buildGraph) GetStaticDependencies(root BuildNode) (result []BuildNode) {
	aliases := root.GetStaticDependencies()
	result = make([]BuildNode, len(aliases))
	for i, alias := range aliases {
		result[i] = g.Find(alias)
	}
	return
}
func (g *buildGraph) GetDynamicDependencies(root BuildNode) (result []BuildNode) {
	aliases := root.GetDynamicDependencies()
	result = make([]BuildNode, len(aliases))
	for i, alias := range aliases {
		result[i] = g.Find(alias)
	}
	return
}
func (g *buildGraph) GetOutputDependencies(root BuildNode) (result []BuildNode) {
	aliases := root.GetOutputDependencies()
	result = make([]BuildNode, len(aliases))
	for i, alias := range aliases {
		result[i] = g.Find(alias)
	}
	return
}

func (g *buildGraph) GetDependencyLinks(a BuildAlias) ([]BuildDependencyLink, error) {
	if node := g.Find(a); node != nil {
		return node.GetDependencyLinks(), nil
	} else {
		return []BuildDependencyLink{}, fmt.Errorf("buildgraph: build node %q not found", a)
	}
}
func (g *buildGraph) GetDependencyInputFiles(alias BuildAlias) (FileSet, error) {
	var files FileSet

	queue := make([]BuildAlias, 0, 32)
	queue = append(queue, alias)

	visiteds := make(map[BuildAlias]int)
	visit := func(node *buildNode) {
		node.state.RLock()
		defer node.state.RUnlock()

		switch file := node.Buildable.(type) {
		case *Filename:
			files.AppendUniq(*file)
		}

		for _, it := range node.Static {
			if _, ok := visiteds[it.Alias]; !ok {
				visiteds[it.Alias] = 1
				queue = append(queue, it.Alias)
			}
		}

		for _, it := range node.Dynamic {
			if _, ok := visiteds[it.Alias]; !ok {
				visiteds[it.Alias] = 1
				queue = append(queue, it.Alias)
			}
		}
	}

	for {
		if len(queue) == 0 {
			break
		}

		a := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		node, _ := g.nodes.Get(a.String())
		if node == nil {
			return files, fmt.Errorf("buildgraph: build node %q not found", a)
		}

		visit(node)
	}

	return files, nil
}

func (g *buildGraph) GetDependencyChain(src, dst BuildAlias) ([]BuildDependencyLink, error) {
	// https://en.wikipedia.org/wiki/Dijkstra%27s_algorithm#:~:text=in%20some%20topologies.-,Pseudocode,-%5Bedit%5D

	const INDEX_NONE int32 = -1

	vertices := g.nodes.Keys()
	previous := make([]int32, len(vertices))
	visiteds := make(map[string]int32, len(vertices))
	distances := make([]int32, len(vertices))
	linkTypes := make([]BuildDependencyType, len(vertices))

	dstIndex := INDEX_NONE
	for i, a := range vertices {
		visiteds[a] = int32(i)
		distances[i] = math.MaxInt32
		previous[i] = INDEX_NONE
		linkTypes[i] = DEPENDENCY_ROOT

		if a == src.String() {
			distances[i] = 0
		} else if a == dst.String() {
			dstIndex = int32(i)
		}
	}

	for len(visiteds) > 0 {
		min := INDEX_NONE
		for _, i := range visiteds {
			if min < 0 || distances[i] < distances[min] {
				min = int32(i)
			}
		}

		u := vertices[min]
		delete(visiteds, u)

		links, err := g.GetDependencyLinks(BuildAlias(u))
		if err != nil {
			return []BuildDependencyLink{}, err
		}

		for _, l := range links {
			v := l.Alias
			if j, ok := visiteds[v.String()]; ok {
				alt := distances[min] + int32(l.Type) // weight by link type, favor output > static > dynamic
				if alt < distances[j] {
					distances[j] = alt
					previous[j] = min
					linkTypes[j] = l.Type
				}
			}
		}
	}

	chain := make([]BuildDependencyLink, distances[dstIndex]+1)
	chain[0] = BuildDependencyLink{
		Alias: dst,
		Type:  DEPENDENCY_ROOT,
	}

	next := dstIndex
	for i := int32(0); i < distances[dstIndex]; i++ {
		next = previous[next]
		chain[i+1] = BuildDependencyLink{
			Alias: BuildAlias(vertices[next]),
			Type:  linkTypes[next],
		}
	}

	return chain, nil
}

func (g *buildGraph) GetBuildStats() BuildStats {
	return g.stats
}
func (g *buildGraph) GetMostExpansiveNodes(n int, inclusive bool) (results []BuildNode) {
	results = make([]BuildNode, 0, n+1)

	predicate := func(i, j int) bool {
		a := results[i].(*buildNode)
		b := results[j].(*buildNode)
		return a.state.stats.Duration.Exclusive > b.state.stats.Duration.Exclusive
	}
	if inclusive {
		predicate = func(i, j int) bool {
			a := results[i].(*buildNode)
			b := results[j].(*buildNode)
			return a.state.stats.Duration.Inclusive > b.state.stats.Duration.Inclusive
		}
	}

	g.nodes.Range(func(key string, node *buildNode) error {
		if node.state.stats.Count != 0 {
			results = append(results, node)
			sort.Slice(results, predicate)

			if len(results) > n {
				results = results[:n]
			}
		}
		return nil
	})
	return
}

func (g *buildGraph) makeDirty() {
	atomic.AddInt32(&g.revision, 1)
}
