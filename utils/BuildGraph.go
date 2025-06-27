package utils

import (
	"context"
	"fmt"
	"io"
	"math"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
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

type BuildStatus byte

const (
	BUILDSTATUS_UNBUILT  BuildStatus = iota // node was not executed at all
	BUILDSTATUS_BUILT                       // node was built but result was identical to previous state
	BUILDSTATUS_UPDATED                     // node was built and result was changed compared to previous state
	BUILDSTATUS_UPTODATE                    // node was not built since all its dependencies were unchanged
)

type BuildResult struct {
	BuildAlias
	BuildStamp
	Buildable Buildable
	Status    BuildStatus
}

type BuildState interface {
	BuildNode
	GetBuildResult() (BuildResult, error)
	GetBuildStats() BuildStats
}
type BuildNodeEvent struct {
	Port BuildGraphWritePort
	Node BuildState
}

type BuildGraphPortFlags byte

const (
	BUILDGRAPH_QUIET BuildGraphPortFlags = iota // do not print a summary when enabled

)

func GetBuildPortFlags() []BuildGraphPortFlags {
	return []BuildGraphPortFlags{
		BUILDGRAPH_QUIET,
	}
}

type BuildGraphReadPort interface {
	base.Closable

	PortName() base.ThreadPoolDebugId
	PortFlags() base.EnumSet[BuildGraphPortFlags, *BuildGraphPortFlags]

	Aliases() BuildAliases
	Expect(alias BuildAlias) (BuildNode, error)
	Range(each func(BuildAlias, BuildNode) error) error

	GetStaticDependencies(BuildNode) []BuildNode
	GetDynamicDependencies(BuildNode) []BuildNode
	GetOutputDependencies(BuildNode) []BuildNode

	GetDependencyChain(a, b BuildAlias, weight func(BuildDependencyLink) float32) ([]BuildDependencyLink, error)
	GetDependencyInputFiles(recursive bool, queue ...BuildAlias) (FileSet, error)
	GetDependencyOutputFiles(queue ...BuildAlias) (FileSet, error)
	GetDependencyLinks(a BuildAlias, includeOutputs bool) ([]BuildDependencyLink, error)
}

type BuildGraphWritePort interface {
	BuildGraphReadPort
	context.Context

	GlobalContext(options ...BuildOptionFunc) BuildContext

	Cancel(error)
	Join() error

	Create(buildable Buildable, staticDeps BuildAliases, options ...BuildOptionFunc) BuildNode
	Build(alias BuildAliasable, options ...BuildOptionFunc) (BuildNode, base.Future[BuildResult])
	BuildMany(aliases BuildAliases, options ...BuildOptionFunc) ([]BuildResult, error)

	GetAggregatedBuildStats() BuildStats
	GetBuildStats(node BuildNode) (BuildStats, bool)
	GetCriticalPathNodes() ([]BuildState, time.Duration)
	GetMostExpansiveNodes(n int, inclusive bool) []BuildState

	RecordSummary(startedAt time.Time) BuildSummary
}

type BuildGraph interface {
	base.Equatable[BuildGraph]
	base.Serializable

	Abort(error)
	Dirty() bool

	Load(io.Reader) error
	PostLoad()
	Save(io.Writer) error

	OpenReadPort(name base.ThreadPoolDebugId, flags ...BuildGraphPortFlags) BuildGraphReadPort
	OpenWritePort(name base.ThreadPoolDebugId, flags ...BuildGraphPortFlags) BuildGraphWritePort

	OnBuildGraphStart(base.EventDelegate[BuildGraphWritePort]) base.DelegateHandle
	OnBuildNodeStart(base.EventDelegate[BuildNodeEvent]) base.DelegateHandle
	OnBuildNodeFinished(base.EventDelegate[BuildNodeEvent]) base.DelegateHandle
	OnBuildGraphFinished(base.EventDelegate[BuildGraphWritePort]) base.DelegateHandle

	RemoveOnBuildGraphStart(base.DelegateHandle) bool
	RemoveOnBuildNodeStart(base.DelegateHandle) bool
	RemoveOnBuildNodeFinished(base.DelegateHandle) bool
	RemoveOnBuildGraphFinished(base.DelegateHandle) bool
}

/***************************************
 * Build Graph
 ***************************************/

type buildGraph struct {
	nodes *base.ShardedMapT[BuildAlias, *buildNode]
	flags *CommandFlags

	portBarrier sync.RWMutex
	dirty       atomic.Bool
	revision    atomic.Int32

	globalContext context.Context
	cancelCause   context.CancelCauseFunc

	buildEvents
}
type buildGraphReadPort struct {
	*buildGraph

	name     base.ThreadPoolDebugId
	flags    base.EnumSet[BuildGraphPortFlags, *BuildGraphPortFlags]
	revision int32
}

type buildGraphWritePortPrivate interface {
	BuildGraphWritePort
	launchBuild(node *buildNode, options *BuildOptions) base.Future[BuildResult]
	buildMany(n int, nodes func(int, *BuildOptions) (*buildNode, error), onResults func(int, BuildResult) error, opts ...BuildOptionFunc) error
	launchBuildMany(n int, nodes func(int, *BuildOptions) (*buildNode, error), opts ...BuildOptionFunc) base.Future[[]BuildResult]
}
type buildGraphWritePort struct {
	buildGraphReadPort

	context.Context
	cancelCause context.CancelCauseFunc

	state base.SharedMapT[BuildAlias, *buildState]
	stats BuildStats

	numRunningTasks atomic.Int32
}

func NewBuildGraph(flags *CommandFlags) BuildGraph {
	result := &buildGraph{
		flags:       flags,
		buildEvents: newBuildEvents(),
		nodes:       base.NewShardedMap[BuildAlias, *buildNode](runtime.NumCPU() + 1),
	}
	result.globalContext, result.cancelCause = context.WithCancelCause(context.Background())
	return result
}

func (g *buildGraph) Abort(err error) {
	if err != nil {
		g.cancelCause(buildAbortError{err})
	}
}

func (g *buildGraph) Dirty() bool {
	return g.dirty.Load()
}
func (g *buildGraph) makeDirty(reason string) {
	if !g.dirty.Swap(true) {
		base.LogWarningVerbose(LogBuildGraph, "graph was dirtied, need to resave before process exit: %s", reason)
	}
}

func (g *buildGraph) OpenReadPort(name base.ThreadPoolDebugId, flags ...BuildGraphPortFlags) BuildGraphReadPort {
	g.portBarrier.RLock()
	readport := buildGraphReadPort{
		buildGraph: g,
		name:       name,
		flags:      base.NewEnumSet(flags...),
		revision:   g.revision.Add(1),
	}
	return &readport
}
func (x *buildGraphReadPort) Close() error {
	defer x.portBarrier.RUnlock()
	x.buildGraph = nil
	return nil
}

func (g *buildGraph) OpenWritePort(name base.ThreadPoolDebugId, flags ...BuildGraphPortFlags) BuildGraphWritePort {
	g.portBarrier.Lock()
	writePort := buildGraphWritePort{
		buildGraphReadPort: buildGraphReadPort{
			buildGraph: g,
			name:       name,
			flags:      base.NewEnumSet(flags...),
			revision:   g.revision.Add(1),
		},
	}
	writePort.Context, writePort.cancelCause = context.WithCancelCause(g.globalContext)
	writePort.onBuildGraphStart_ThreadSafe()
	return &writePort
}
func (x *buildGraphWritePort) Cancel(err error) {
	if err != nil {
		x.cancelCause(err)
	}
}
func (x *buildGraphWritePort) Close() (err error) {
	defer x.portBarrier.Unlock()
	err = x.Join()
	x.cancelCause(nil)
	x.onBuildGraphFinished_ThreadSafe()
	x.buildGraph = nil
	x.Context = nil
	x.cancelCause = nil
	return
}

func (g *buildGraph) PostLoad() {
	if g.flags.Purge.Get() {
		g.nodes.Clear()
		g.makeDirty("purged due to `-F` command-line option")
	}
}
func (g *buildGraph) Serialize(ar base.Archive) {
	var pinned []*buildNode
	serialize := func(node **buildNode) {
		*node = new(buildNode)
		ar.Serializable(*node)
	}
	if !ar.Flags().Has(base.AR_LOADING) {
		serialize = func(node **buildNode) {
			ar.Serializable(*node)
		}
		pinned = g.nodes.Values()
		sort.Slice(pinned, func(i, j int) bool {
			return pinned[i].BuildAlias.Compare(pinned[j].BuildAlias) < 0
		})
	}
	base.SerializeMany(ar, serialize, &pinned)
	if ar.Flags().Has(base.AR_LOADING) && ar.Error() == nil {
		g.nodes.Clear()
		g.dirty.Store(false)

		for _, node := range pinned {
			g.nodes.Add(node.Alias(), node)
		}
	}
}
func (g *buildGraph) Save(dst io.Writer) (err error) {
	if err = base.CompressedArchiveFileWrite(dst, g.Serialize, base.TransientPage64KiB, base.TASKPRIORITY_HIGH, base.AR_FLAGS_NONE); err == nil {
		g.dirty.Store(false)
	}
	return
}
func (g *buildGraph) Load(src io.Reader) error {
	archiveFlags := base.AR_FLAGS_NONE
	if g.flags.Force.Get() || g.flags.Purge.Get() {
		archiveFlags = base.AR_FLAGS_TOLERANT
	}
	file, err := base.CompressedArchiveFileRead(g.globalContext, src, g.Serialize, base.TransientPage64KiB, base.TASKPRIORITY_HIGH, archiveFlags)
	base.LogVeryVerbose(LogBuildGraph, "archive version = %v tags = %v", file.Version, file.Tags)
	return err
}

func (g *buildGraph) Equals(other BuildGraph) bool {
	return other.(*buildGraph) == g
}

/***************************************
 * Build Graph Read Port
 ***************************************/

func (g *buildGraphReadPort) PortName() base.ThreadPoolDebugId {
	return g.name
}
func (g *buildGraphReadPort) PortFlags() base.EnumSet[BuildGraphPortFlags, *BuildGraphPortFlags] {
	return g.flags
}

func (g *buildGraphReadPort) Aliases() (result BuildAliases) {
	result = g.nodes.Keys()
	sort.Slice(result, func(i, j int) bool {
		return result[i].Compare(result[j]) < 0
	})
	return
}
func (g *buildGraphReadPort) Range(each func(BuildAlias, BuildNode) error) error {
	return g.nodes.Range(func(key BuildAlias, node *buildNode) error {
		return each(key, node)
	})
}

func ForeachBuildable[T Buildable](bg BuildGraphReadPort, each func(BuildAlias, T) error) error {
	return bg.Range(func(alias BuildAlias, node BuildNode) error {
		if buildable, ok := node.GetBuildable().(T); ok {
			if err := each(alias, buildable); err != nil {
				return err
			}
		}
		return nil
	})
}

func (g *buildGraphReadPort) findNode(alias BuildAlias) (*buildNode, error) {
	if node, ok := g.nodes.Get(alias); ok {
		base.Assert(func() bool { return node.Alias().Equals(alias) })
		return node, nil
	} else {
		base.LogTrace(LogBuildGraph, "Find(%q) -> NOT_FOUND", alias)
		return nil, BuildableNotFound{Alias: alias}
	}
}

func (g *buildGraphReadPort) Expect(alias BuildAlias) (node BuildNode, err error) {
	node, err = g.findNode(alias)
	return
}

func (g *buildGraphReadPort) GetStaticDependencies(root BuildNode) (result []BuildNode) {
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
func (g *buildGraphReadPort) GetDynamicDependencies(root BuildNode) (result []BuildNode) {
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
func (g *buildGraphReadPort) GetOutputDependencies(root BuildNode) (result []BuildNode) {
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

func (g *buildGraphReadPort) GetDependencyLinks(a BuildAlias, includeOutputs bool) ([]BuildDependencyLink, error) {
	if node, err := g.Expect(a); err == nil {
		return node.GetDependencyLinks(includeOutputs), nil
	} else {
		return []BuildDependencyLink{}, err
	}
}
func (g *buildGraphReadPort) GetDependencyInputFiles(recursive bool, queue ...BuildAlias) (FileSet, error) {
	var files FileSet

	visiteds := make(map[BuildAlias]bool, 32)
	visit := func(node *buildNode, recursive bool) {
		node.RLock()
		defer node.RUnlock()

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
func (g *buildGraphReadPort) GetDependencyOutputFiles(queue ...BuildAlias) (FileSet, error) {
	var files FileSet

	visiteds := make(map[BuildAlias]bool, 32)
	visit := func(node *buildNode) {
		node.RLock()
		defer node.RUnlock()

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

func (g *buildGraphReadPort) GetDependencyChain(src, dst BuildAlias, weight func(BuildDependencyLink) float32) ([]BuildDependencyLink, error) {
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

		switch a {
		case src:
			distances[i] = 0
			srcIndex = int32(i)
		case dst:
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

		if links, err := g.GetDependencyLinks(BuildAlias(u), false); err == nil {
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
		} else {
			return []BuildDependencyLink{}, err
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

/***************************************
 * Build Graph Write Port
 ***************************************/

func (g *buildGraphWritePort) GlobalContext(options ...BuildOptionFunc) BuildContext {
	bo := NewBuildOptions(options...)
	context := makeBuildGraphContext(g, &bo)
	return &context
}

func (g *buildGraphWritePort) Create(buildable Buildable, static BuildAliases, options ...BuildOptionFunc) BuildNode {
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

	node.Lock()
	defer node.Unlock()

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

func (x *buildGraphWritePort) CheckForAbort() error {
	if err := x.Context.Err(); err != nil {
		return buildAbortError{context.Cause(x.Context)}
	}
	return nil
}

func (g *buildGraphWritePort) Build(it BuildAliasable, options ...BuildOptionFunc) (BuildNode, base.Future[BuildResult]) {
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
		return nil, base.MakeFutureError[BuildResult](err)
	}
}

func (g *buildGraphWritePort) BuildMany(targets BuildAliases, options ...BuildOptionFunc) (results []BuildResult, err error) {
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

func (g *buildGraphWritePort) hasRunningTasks() bool {
	return g.numRunningTasks.Load() > 0
}
func (g *buildGraphWritePort) Join() (err error) {
	base.JoinAllThreadPools()
	for {
		err = g.state.Range(func(_ BuildAlias, state *buildState) error {
			if future := state.future.Load(); future != nil {
				return future.Join().Failure()
			}
			return nil
		})
		if err != nil {
			return
		}
		if !g.hasRunningTasks() {
			break
		}
	}
	return
}

func (g *buildGraphWritePort) GetAggregatedBuildStats() BuildStats {
	return g.stats
}
func (g *buildGraphWritePort) GetBuildStats(node BuildNode) (BuildStats, bool) {
	if state, ok := g.state.Get(node.Alias()); ok {
		return state.stats, true
	} else {
		return BuildStats{}, false
	}
}
func (g *buildGraphWritePort) GetCriticalPathNodes() ([]BuildState, time.Duration) {
	defer base.LogBenchmark(LogBuildGraph, "GetCriticalPathNodes(%v)", g.name).Close()

	type criticalPath struct {
		Path     base.SetT[BuildState]
		Duration time.Duration
	}

	var dfs func(*buildState) criticalPath
	visited := make(map[BuildAlias]criticalPath, g.state.Len())
	foreachDep := func(alias BuildAlias, longest criticalPath) criticalPath {
		var path criticalPath
		if recorded, ok := visited[alias]; ok {
			path = recorded
		} else if dep, ok := g.state.Get(alias); ok {
			path = dfs(dep)
		}
		if path.Duration > longest.Duration {
			return path
		} else {
			return longest
		}
	}

	// use previous node as root to look for the longest execution path using depth first search
	dfs = func(node *buildState) criticalPath {
		var longest criticalPath

		for _, it := range node.buildNode.Static {
			longest = foreachDep(it.Alias, longest)
		}
		for _, it := range node.buildNode.Dynamic {
			longest = foreachDep(it.Alias, longest)
		}

		longest.Duration += node.stats.Duration.Exclusive
		longest.Path = append([]BuildState{node}, longest.Path...)
		visited[node.BuildAlias] = longest

		return longest
	}

	var longest criticalPath
	g.state.Range(func(alias BuildAlias, state *buildState) error {
		longest = foreachDep(alias, longest)
		return nil
	})

	if len(longest.Path) > 0 {
		// return true duration of execution, instead of accumulated inclusive time
		var startedAt, finishedAt time.Duration
		for i, it := range longest.Path {
			stats := it.GetBuildStats()
			if t := stats.InclusiveStart; i == 0 || t < startedAt {
				startedAt = t
			}
			if t := stats.GetInclusiveEnd(); i == 0 || t > finishedAt {
				finishedAt = t
			}
		}

		longest.Duration = finishedAt - startedAt
	}

	return longest.Path, longest.Duration
}
func (g *buildGraphWritePort) GetMostExpansiveNodes(n int, inclusive bool) []BuildState {
	results := make([]BuildState, 0, n)

	predicate := func(a, b BuildState) bool {
		return a.GetBuildStats().Duration.Exclusive > b.GetBuildStats().Duration.Exclusive
	}
	if inclusive {
		predicate = func(a, b BuildState) bool {
			return a.GetBuildStats().Duration.Inclusive > b.GetBuildStats().Duration.Inclusive
		}
	}

	base.LogPanicIfFailed(LogBuildGraph, g.state.Range(func(_ BuildAlias, state *buildState) error {
		if state.stats.Count != 0 {
			results = base.AppendBoundedSort(results, n, BuildState(state), predicate)
		}
		return nil
	}))

	return results
}

/***************************************
 * Build Node Status
 ***************************************/

func (x BuildStatus) WasUpdated() bool {
	switch x {
	case BUILDSTATUS_UPDATED:
		return true
	default:
		return false
	}
}

/***************************************
 * Build Graph Read/Write Port Flags
 ***************************************/

func (x BuildGraphPortFlags) Ord() int32 { return int32(byte(x)) }
func (x BuildGraphPortFlags) Mask() int32 {
	return base.EnumBitMask(GetBuildPortFlags()...)
}
func (x *BuildGraphPortFlags) FromOrd(v int32) { *x = BuildGraphPortFlags(v) }
func (x BuildGraphPortFlags) String() string {
	switch x {
	case BUILDGRAPH_QUIET:
		return "QUIET"
	}
	base.UnexpectedValue(x)
	return ""
}
func (x *BuildGraphPortFlags) Set(in string) error {
	switch strings.ToUpper(in) {
	case BUILDGRAPH_QUIET.String():
		*x = BUILDGRAPH_QUIET
	default:
		return base.MakeUnexpectedValueError(x, in)
	}
	return nil
}
func (x BuildGraphPortFlags) Description() string {
	switch x {
	case BUILDGRAPH_QUIET:
		return "won't print summary when build finished"
	}
	base.UnexpectedValue(x)
	return ""
}
func (x BuildGraphPortFlags) AutoComplete(in base.AutoComplete) {
	in.Add(BUILDGRAPH_QUIET.String(), BUILDGRAPH_QUIET.Description())
}
