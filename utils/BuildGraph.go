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
	BuildAlias
	BuildStamp
	Buildable Buildable
}

type BuildNodeEvent struct {
	Alias     BuildAlias
	Buildable Buildable
}

type BuildGraph interface {
	Aliases() BuildAliases
	Range(each func(BuildAlias, BuildNode) error) error

	Dirty() bool

	GlobalContext(options ...BuildOptionFunc) BuildContext

	Expect(alias BuildAlias) (BuildNode, error)
	Create(buildable Buildable, staticDeps BuildAliases, options ...BuildOptionFunc) BuildNode
	Build(alias BuildAliasable, options ...BuildOptionFunc) (BuildNode, base.Future[BuildResult])
	BuildMany(aliases BuildAliases, options ...BuildOptionFunc) ([]BuildResult, error)

	Abort(error)
	Join() error

	Load(io.Reader) error
	PostLoad()
	Save(io.Writer) error

	GetStaticDependencies(BuildNode) []BuildNode
	GetDynamicDependencies(BuildNode) []BuildNode
	GetOutputDependencies(BuildNode) []BuildNode

	GetDependencyChain(a, b BuildAlias, weight func(BuildDependencyLink) float32) ([]BuildDependencyLink, error)
	GetDependencyInputFiles(recursive bool, queue ...BuildAlias) (FileSet, error)
	GetDependencyOutputFiles(queue ...BuildAlias) (FileSet, error)
	GetDependencyLinks(BuildAlias) ([]BuildDependencyLink, error)

	GetBuildStats() BuildStats
	GetCriticalPathNodes() []BuildNode
	GetMostExpansiveNodes(n int, inclusive bool) []BuildNode

	PrintSummary(startedAt time.Time, level base.LogLevel)

	OnBuildGraphStart(base.EventDelegate[BuildGraph]) base.DelegateHandle
	OnBuildNodeStart(base.EventDelegate[BuildNode]) base.DelegateHandle
	OnBuildNodeFinished(base.EventDelegate[BuildNode]) base.DelegateHandle
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
	nodes   *base.ShardedMapT[BuildAlias, *buildNode]
	options BuildOptions
	stats   BuildStats

	revision int32
	abort    atomic.Value

	buildEvents
}

func NewBuildGraph(flags *CommandFlags, options ...BuildOptionFunc) BuildGraph {
	result := &buildGraph{
		flags:       flags,
		nodes:       base.NewShardedMap[BuildAlias, *buildNode](1000),
		options:     NewBuildOptions(options...),
		revision:    0,
		buildEvents: newBuildEvents(),
	}
	return result
}

func (g *buildGraph) Aliases() (result BuildAliases) {
	result = g.nodes.Keys()
	sort.Slice(result, func(i, j int) bool {
		return result[i].Compare(result[j]) < 0
	})
	return
}
func (g *buildGraph) Range(each func(BuildAlias, BuildNode) error) error {
	return g.nodes.Range(func(key BuildAlias, node *buildNode) error {
		return each(key, node)
	})
}

func (g *buildGraph) Dirty() bool {
	return atomic.LoadInt32(&g.revision) > 0
}

func (g *buildGraph) GlobalContext(options ...BuildOptionFunc) BuildContext {
	bo := NewBuildOptions(options...)
	context := makeBuildGraphContext(g, &bo)
	return &context
}

func (g *buildGraph) findNode(alias BuildAlias) (*buildNode, error) {
	if node, ok := g.nodes.Get(alias); ok {
		base.Assert(func() bool { return node.Alias().Equals(alias) })
		return node, nil
	} else {
		base.LogTrace(LogBuildGraph, "Find(%q) -> NOT_FOUND", alias)
		return nil, BuildableNotFound{Alias: alias}
	}
}

func (g *buildGraph) Expect(alias BuildAlias) (node BuildNode, err error) {
	node, err = g.findNode(alias)
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

	if node, loaded = g.nodes.Get(alias); !loaded {
		base.Assert(func() bool {
			makeCaseInsensitive := func(in string) string {
				return SanitizePath(strings.ToLower(in), '/')
			}
			lowerAlias := makeCaseInsensitive(alias.String())
			for _, key := range g.nodes.Keys() {
				lowerKey := makeCaseInsensitive(key.String())
				if lowerKey == lowerAlias {
					if key != alias {
						base.LogError(LogBuildGraph, "alias already registered with different case:\n\t add: %v\n\tfound: %v", alias, key)
						return false
					}
					break
				}
			}
			return true
		})

		// first optimistic Get() to avoid newBuildNode() bellow
		node, loaded = g.nodes.FindOrAdd(alias, newBuildNode(alias, buildable))
	}

	// quick reject if a node with same alias already exists
	if loaded && !(bo.Force || bo.Dirty) {
		return node
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

		g.makeDirty("created dirty node")
	}

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

	if node, err := g.findNode(a); err == nil {
		bo := NewBuildOptions(options...)
		return node, g.launchBuild(node, &bo)
	} else {
		return nil, base.MakeFutureError[BuildResult](fmt.Errorf("build: unknown node %q", a))
	}
}

func (g *buildGraph) BuildMany(targets BuildAliases, options ...BuildOptionFunc) (results []BuildResult, err error) {
	results = make([]BuildResult, targets.Len())
	err = g.buildMany(targets.Len(),
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return g.findNode(targets[i])
		},
		func(i int, br BuildResult) error {
			results[i] = br
			return nil
		},
		options...)
	return
}

func (g *buildGraph) Join() (lastErr error) {
	for lastErr == nil && g.hasRunningTasks() {
		lastErr = g.nodes.Range(func(_ BuildAlias, node *buildNode) error {
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
func (g *buildGraph) Abort(err error) {
	if err != nil {
		// only keeps the first error
		var null error = nil
		g.abort.CompareAndSwap(null, err)
	}
}

func (g *buildGraph) PostLoad() {
	if g.flags.Purge.Get() {
		g.revision = 0
		g.nodes.Clear()
		g.makeDirty("clean purge requested")
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
			g.nodes.Add(node.Alias(), node)
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
		var err error
		if result[i], err = g.Expect(alias); err != nil {
			base.LogPanicErr(LogBuildGraph, err)
		}
	}
	return
}
func (g *buildGraph) GetDynamicDependencies(root BuildNode) (result []BuildNode) {
	aliases := root.GetDynamicDependencies()
	result = make([]BuildNode, len(aliases))
	for i, alias := range aliases {
		var err error
		if result[i], err = g.Expect(alias); err != nil {
			base.LogPanicErr(LogBuildGraph, err)
		}
	}
	return
}
func (g *buildGraph) GetOutputDependencies(root BuildNode) (result []BuildNode) {
	aliases := root.GetOutputDependencies()
	result = make([]BuildNode, len(aliases))
	for i, alias := range aliases {
		var err error
		if result[i], err = g.Expect(alias); err != nil {
			base.LogPanicErr(LogBuildGraph, err)
		}
	}
	return
}

func (g *buildGraph) GetDependencyLinks(a BuildAlias) ([]BuildDependencyLink, error) {
	if node, err := g.Expect(a); err == nil {
		return node.GetDependencyLinks(), nil
	} else {
		return []BuildDependencyLink{}, err
	}
}
func (g *buildGraph) GetDependencyInputFiles(recursive bool, queue ...BuildAlias) (FileSet, error) {
	var files FileSet

	visiteds := make(map[BuildAlias]bool, 32)
	visit := func(node *buildNode, recursive bool) {
		node.state.RLock()
		defer node.state.RUnlock()

		switch file := node.Buildable.(type) {
		case BuildableSourceFile:
			files.AppendUniq(file.GetSourceFile())
		case BuildableGeneratedFile:
			files.AppendUniq(file.GetGeneratedFile())
		}

		if recursive {
			for _, it := range node.Static {
				if _, ok := visiteds[it.Alias]; !ok {
					visiteds[it.Alias] = true
					queue = append(queue, it.Alias)
				}
			}

			for _, it := range node.Dynamic {
				if _, ok := visiteds[it.Alias]; !ok {
					visiteds[it.Alias] = true
					queue = append(queue, it.Alias)
				}
			}
		}
	}

	for lenUserInput := len(queue); len(queue) > 0; lenUserInput-- {
		a := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		node, err := g.Expect(a)
		if err != nil {
			return FileSet{}, err
		}

		visit(node.(*buildNode), recursive || lenUserInput > 0)
	}

	return files, nil
}
func (g *buildGraph) GetDependencyOutputFiles(queue ...BuildAlias) (FileSet, error) {
	var files FileSet

	visiteds := make(map[BuildAlias]bool, 32)
	visit := func(node *buildNode) {
		node.state.RLock()
		defer node.state.RUnlock()

		switch file := node.Buildable.(type) {
		case BuildableSourceFile:
			files.AppendUniq(file.GetSourceFile())
		case BuildableGeneratedFile:
			files.AppendUniq(file.GetGeneratedFile())
		}

		for _, it := range node.OutputFiles {
			if _, ok := visiteds[it.Alias]; !ok {
				visiteds[it.Alias] = true
				queue = append(queue, it.Alias)
			}
		}

		for _, alias := range node.OutputNodes {
			if _, ok := visiteds[alias]; !ok {
				visiteds[alias] = true
				queue = append(queue, alias)
			}
		}
	}

	for len(queue) > 0 {
		a := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		node, err := g.Expect(a)
		if err != nil {
			return FileSet{}, err
		}

		visit(node.(*buildNode))
	}

	return files, nil
}

func (g *buildGraph) GetDependencyChain(src, dst BuildAlias, weight func(BuildDependencyLink) float32) ([]BuildDependencyLink, error) {
	// https://en.wikipedia.org/wiki/Dijkstra%27s_algorithm#:~:text=in%20some%20topologies.-,Pseudocode,-%5Bedit%5D

	const INDEX_NONE int32 = -1

	vertices := g.nodes.Keys()
	if len(vertices) == 0 {
		return []BuildDependencyLink{}, nil
	}

	var (
		dstIndex  = INDEX_NONE
		srcIndex  = INDEX_NONE
		previous  = make([]int32, len(vertices))
		visiteds  = make(map[BuildAlias]int32, len(vertices))
		distances = make([]float32, len(vertices))
		linkTypes = make([]BuildDependencyType, len(vertices))
	)

	for i, a := range vertices {
		visiteds[a] = int32(i)
		distances[i] = math.MaxFloat32
		previous[i] = INDEX_NONE
		linkTypes[i] = DEPENDENCY_ROOT

		if a == src {
			distances[i] = 0
			srcIndex = int32(i)
		} else if a == dst {
			dstIndex = int32(i)
		}
	}

	for len(visiteds) > 0 {
		min := INDEX_NONE
		for _, i := range visiteds {
			if min == INDEX_NONE || distances[i] < distances[min] {
				min = int32(i)
			}
		}

		if distances[min] == math.MaxFloat32 {
			break // no more reachability
		}

		u := vertices[min]
		delete(visiteds, u)

		if u == dst {
			break // destination found, guaranteed with shortest path
		}

		links, err := g.GetDependencyLinks(BuildAlias(u))
		if err != nil {
			return []BuildDependencyLink{}, err
		}

		for _, l := range links {
			v := l.Alias
			if j, ok := visiteds[v]; ok {
				if alt := distances[min] + 1; alt < distances[j] {
					distances[j] = alt
					previous[j] = min
					linkTypes[j] = l.Type
				}
			}
		}
	}

	if distances[dstIndex] == math.MaxFloat32 {
		return []BuildDependencyLink{}, fmt.Errorf("no link found between %q and %q", src, dst)
	}

	chain := make([]BuildDependencyLink, 1, 2)
	chain[0] = BuildDependencyLink{
		Alias: dst,
		Type:  DEPENDENCY_ROOT,
	}

	for index := dstIndex; index != srcIndex; {
		index = previous[index]
		chain = append(chain, BuildDependencyLink{
			Alias: BuildAlias(vertices[index]),
			Type:  linkTypes[index],
		})
	}

	return chain, nil
}

func (g *buildGraph) GetBuildStats() BuildStats {
	return g.stats
}
func (g *buildGraph) GetCriticalPathNodes() []BuildNode {
	type buildNodes = base.SetT[BuildNode]
	type criticalPath struct {
		Nodes             buildNodes
		InclusiveDuration time.Duration
	}

	var critical criticalPath

	base.LogPanicIfFailed(LogBuildGraph,
		g.nodes.Range(func(_ BuildAlias, rootNode *buildNode) error {
			if rootNode.GetBuildStats().Count == 0 ||
				critical.Nodes.Contains(rootNode) {
				return nil
			}

			queue := make([]criticalPath, 1)
			queue[0] = criticalPath{Nodes: buildNodes{rootNode}}

			foreachDependency := func(path criticalPath, it BuildAlias) error {
				if node, err := g.Expect(it); err == nil {
					base.Assert(func() bool { return !path.Nodes.Contains(node) })
					queue = append(queue, criticalPath{
						Nodes:             append(path.Nodes, node),
						InclusiveDuration: path.InclusiveDuration,
					})
					return nil
				} else {
					return err
				}
			}

			for len(queue) > 0 {
				path := queue[len(queue)-1]
				queue = queue[:len(queue)-1]

				tip := path.Nodes[len(path.Nodes)-1]

				path.InclusiveDuration += tip.GetBuildStats().Duration.Exclusive
				if path.InclusiveDuration > critical.InclusiveDuration {
					critical = path
				}

				for _, it := range tip.GetStaticDependencies() {
					if err := foreachDependency(path, it); err != nil {
						return err
					}
				}

				for _, it := range tip.GetDynamicDependencies() {
					if err := foreachDependency(path, it); err != nil {
						return err
					}
				}
			}

			return nil
		}))

	return critical.Nodes
}
func (g *buildGraph) GetMostExpansiveNodes(n int, inclusive bool) (results []BuildNode) {
	results = make([]BuildNode, 0, n)

	predicate := func(a, b BuildNode) bool {
		return a.(*buildNode).state.stats.Duration.Exclusive > b.(*buildNode).state.stats.Duration.Exclusive
	}
	if inclusive {
		predicate = func(a, b BuildNode) bool {
			return a.(*buildNode).state.stats.Duration.Inclusive > b.(*buildNode).state.stats.Duration.Inclusive
		}
	}

	err := g.nodes.Range(func(key BuildAlias, node *buildNode) error {
		if node.state.stats.Count != 0 {
			results = base.AppendBoundedSort(results, n, BuildNode(node), predicate)
		}
		return nil
	})
	base.LogPanicIfFailed(LogBuildGraph, err)
	return
}

func (g *buildGraph) makeDirty(reason string) {
	if atomic.AddInt32(&g.revision, 1) == 1 {
		base.LogWarningVerbose(LogBuildGraph, "graph was dirtied, need to resave after execution: %s", reason)
	}
}
