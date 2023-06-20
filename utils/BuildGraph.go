package utils

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const BUILDGRAPH_ENABLE_CHECKS = false // %_NOCOMMIT%

/***************************************
 * Public API
 ***************************************/

var LogBuildGraph = NewLogCategory("BuildGraph")

type BuildDependencyType int32

const (
	DEPENDENCY_ROOT   BuildDependencyType = -1
	DEPENDENCY_OUTPUT BuildDependencyType = iota
	DEPENDENCY_STATIC
	DEPENDENCY_DYNAMIC
)

type BuildDependencyLink struct {
	Alias BuildAlias
	Type  BuildDependencyType
}

type BuildAlias string
type BuildAliases = SetT[BuildAlias]

type BuildAliasable interface {
	Alias() BuildAlias
}

type Buildable interface {
	BuildAliasable
	Serializable
	Build(BuildContext) error
}

type BuildStamp struct {
	ModTime time.Time
	Content Fingerprint
}

type BuildResult struct {
	Buildable
	BuildStamp
}

type BuildStats struct {
	InclusiveStart time.Duration
	ExclusiveStart time.Duration
	Duration       struct {
		Inclusive time.Duration
		Exclusive time.Duration
	}
	Count int32
}

type BuildDependency struct {
	Alias BuildAlias
	Stamp BuildStamp
}

type BuildDependencies []BuildDependency

type BuildNode interface {
	BuildAliasable

	GetBuildStamp() BuildStamp
	GetBuildStats() BuildStats
	GetBuildable() Buildable

	DependsOn(...BuildAlias) bool

	GetStaticDependencies() BuildAliases
	GetDynamicDependencies() BuildAliases
	GetOutputDependencies() BuildAliases

	GetDependencyLinks() []BuildDependencyLink
}

type BuildOptions struct {
	Parent                   *BuildOptions
	Caller                   BuildNode
	OnLaunched               PublicEvent[BuildNode]
	OnBuilt                  PublicEvent[BuildNode]
	Stamp                    *BuildStamp
	Dirty                    bool
	Force                    bool
	Recursive                bool
	NoWarningOnMissingOutput bool
}

type BuildOptionFunc func(*BuildOptions)

type BuildGraph interface {
	Aliases() []BuildAlias
	Dirty() bool

	GlobalContext(options ...BuildOptionFunc) BuildContext

	Find(alias BuildAlias) BuildNode
	Create(buildable Buildable, staticDeps BuildAliases, options ...BuildOptionFunc) BuildNode
	Build(alias BuildAliasable, options ...BuildOptionFunc) (BuildNode, Future[BuildResult])
	BuildMany(aliases BuildAliases, options ...BuildOptionFunc) Future[[]BuildResult]
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

	OnBuildGraphStart() MutableEvent[BuildGraph]
	OnBuildNodeStart() MutableEvent[BuildNode]
	OnBuildNodeFinished() MutableEvent[BuildNode]
	OnBuildGraphFinished() MutableEvent[BuildGraph]

	Equatable[BuildGraph]
	Serializable
}

type BuildInitializer interface {
	BuildGraph() BuildGraph
	Options() *BuildOptions

	DependsOn(...BuildAlias) error

	NeedFactory(BuildFactory, ...BuildOptionFunc) (Buildable, error)

	NeedFactories(...BuildFactory) error
	NeedBuildable(...BuildAliasable) error
	NeedFile(...Filename) error
	NeedDirectory(...Directory) error
}

type BuildContext interface {
	BuildInitializer

	OutputFile(...Filename) error
	OutputNode(...BuildFactory) error

	OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error)

	Annotate(string)
	Timestamp(time.Time)

	OnBuilt(func(BuildNode) error)
}

type BuildFactory interface {
	Create(BuildInitializer) (Buildable, error)
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
	future AtomicFuture[BuildResult]
}

func newBuildNode(alias BuildAlias, builder Buildable) *buildNode {
	Assert(func() bool { return alias.Valid() })
	Assert(func() bool { return builder.Alias().Equals(alias) })
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

		future: AtomicFuture[BuildResult]{},
	}
}
func (node *buildNode) Alias() BuildAlias { return node.BuildAlias }
func (node *buildNode) String() string    { return node.BuildAlias.String() }

func (node *buildNode) IsFile() bool {
	node.state.RLock()
	defer node.state.RUnlock()
	if _, ok := node.Buildable.(*Filename); ok {
		return true
	}
	if _, ok := node.Buildable.(*Directory); ok {
		return true
	}
	if strings.HasPrefix(string(node.BuildAlias), "UFS://") {
		return true
	}
	return false
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
func (node *buildNode) GetDependencyLinks() []BuildDependencyLink {
	node.state.RLock()
	defer node.state.RUnlock()
	result := make([]BuildDependencyLink, 0, len(node.Static)+len(node.Dynamic)+len(node.OutputFiles)+len(node.OutputNodes))
	for _, it := range node.Static {
		result = append(result, BuildDependencyLink{Alias: it.Alias, Type: DEPENDENCY_STATIC})
	}
	for _, it := range node.Dynamic {
		result = append(result, BuildDependencyLink{Alias: it.Alias, Type: DEPENDENCY_DYNAMIC})
	}
	for _, it := range node.OutputFiles {
		result = append(result, BuildDependencyLink{Alias: it.Alias, Type: DEPENDENCY_OUTPUT})
	}
	for _, a := range node.OutputNodes {
		result = append(result, BuildDependencyLink{Alias: a, Type: DEPENDENCY_OUTPUT})
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
func (node *buildNode) Serialize(ar Archive) {
	ar.Serializable(&node.BuildAlias)
	SerializeExternal(ar, &node.Buildable)

	ar.Serializable(&node.Stamp)

	ar.Serializable(&node.Static)
	ar.Serializable(&node.Dynamic)
	ar.Serializable(&node.OutputFiles)
	SerializeSlice(ar, node.OutputNodes.Ref())
}

func (node *buildNode) makeDirty_AssumeLocked() {
	node.Dynamic = BuildDependencies{}
	node.OutputFiles = BuildDependencies{}
	node.OutputNodes = BuildAliases{}
}

func (node *buildNode) addStatic_AssumeLocked(a BuildAlias, stamp BuildStamp) {
	AssertMessage(func() bool { return a.Valid() }, "%v: invalid empty alias for static dependency")
	AssertMessage(func() bool { return !node.Alias().Equals(a) }, "%v: can't have a static dependency to self", a)
	AssertMessage(func() bool { _, ok := node.Dynamic.IndexOf(a); return !ok }, "%v: static dependency is already dynamic <%v>", node, a)
	AssertMessage(func() bool { _, ok := node.OutputFiles.IndexOf(a); return !ok }, "%v: static dependency is already output <%v>", node, a)
	AssertMessage(func() bool { ok := node.OutputNodes.Contains(a); return !ok }, "%v: static dependency is already output <%v>", node, a)

	node.Static.Add(a, stamp)
}
func (node *buildNode) addDynamic_AssumeLocked(a BuildAlias, stamp BuildStamp) {
	Assert(func() bool { return stamp.Content.Valid() })
	AssertMessage(func() bool { return a.Valid() }, "%v: invalid empty alias for dynamic dependency")
	AssertMessage(func() bool { return !node.Alias().Equals(a) }, "%v: can't have an output dependency to self", a)
	AssertMessage(func() bool { _, ok := node.Static.IndexOf(a); return !ok }, "%v: dynamic dependency is already static <%v>", node, a)
	AssertMessage(func() bool { _, ok := node.OutputFiles.IndexOf(a); return !ok }, "%v: dynamic dependency is already output <%v>", node, a)
	AssertMessage(func() bool { ok := node.OutputNodes.Contains(a); return !ok }, "%v: dynamic dependency is already output <%v>", node, a)

	node.Dynamic.Add(a, stamp)
}
func (node *buildNode) addOutputFile_AssumeLocked(a BuildAlias, stamp BuildStamp) {
	Assert(func() bool { return stamp.Content.Valid() })
	AssertMessage(func() bool { return a.Valid() }, "%v: invalid empty alias for output file dependency")
	AssertMessage(func() bool { return !node.Alias().Equals(a) }, "%v: can't have an output dependency to self", a)
	AssertMessage(func() bool { _, ok := node.Static.IndexOf(a); return !ok }, "%v: output file dependency is already static <%v>", node, a)
	AssertMessage(func() bool { _, ok := node.Dynamic.IndexOf(a); return !ok }, "%v: output file dependency is already dynamic <%v>", node, a)
	AssertMessage(func() bool { ok := node.OutputNodes.Contains(a); return !ok }, "%v: output file dependency is already output node <%v>", node, a)

	node.OutputFiles.Add(a, stamp)
}
func (node *buildNode) addOutputNode_AssumeLocked(a BuildAlias) {
	AssertMessage(func() bool { return a.Valid() }, "%v: invalid empty alias for output dependency")
	AssertMessage(func() bool { return !node.Alias().Equals(a) }, "%v: can't have an output dependency to self", a)
	AssertMessage(func() bool { _, ok := node.Static.IndexOf(a); return !ok }, "%v: output node is already static dependency <%v>", node, a)
	AssertMessage(func() bool { _, ok := node.Dynamic.IndexOf(a); return !ok }, "%v: output node is already dynamic dependency <%v>", node, a)
	AssertMessage(func() bool { _, ok := node.OutputFiles.IndexOf(a); return !ok }, "%v: output node dependency is already output file dependency <%v>", node, a)

	node.OutputNodes.AppendUniq(a)
}

/***************************************
 * Build Factory Typed
 ***************************************/

type BuildableNotFound struct {
	Alias BuildAlias
}

func (x BuildableNotFound) Error() string {
	return fmt.Sprintf("buildable not found: %q", x.Alias)
}

func FindGlobalBuildable[T Buildable](alias BuildAlias) (result T, err error) {
	return FindBuildable[T](CommandEnv.buildGraph, alias)
}
func FindBuildable[T Buildable](graph BuildGraph, alias BuildAlias) (result T, err error) {
	if node := graph.Find(alias); node != nil {
		result = node.GetBuildable().(T)
	} else {
		err = BuildableNotFound{Alias: alias}
	}
	return
}

type BuildFactoryTyped[T Buildable] interface {
	BuildFactory

	Need(BuildInitializer, ...BuildOptionFunc) (T, error)
	SafeNeed(BuildInitializer, ...BuildOptionFunc) T
	Output(BuildContext, ...BuildOptionFunc) (T, error)

	Init(BuildGraph, ...BuildOptionFunc) (T, error)
	Prepare(BuildGraph, ...BuildOptionFunc) Future[T]
	Build(BuildGraph, ...BuildOptionFunc) Result[T]
}

func MakeBuildFactory[T any, B interface {
	*T
	Buildable
}](factory func(BuildInitializer) (T, error)) BuildFactoryTyped[B] {
	return WrapBuildFactory(func(bi BuildInitializer) (B, error) {
		// #TODO: refactor to avoid allocation when possible
		value, err := factory(bi)
		return B(&value), err
		// if value, err := factory(bi); err == nil {
		// 	if !bi.Options().Force {
		// 		alias := B(&value).Alias()
		// 		if node := CommandEnv.buildGraph.Find(alias); node != nil {
		// 			return node.GetBuildable().(B), nil
		// 		}
		// 	}
		// 	return B(&value), nil

		// } else {
		// 	return nil, err
		// }
	})
}

type buildFactoryWrapped[T Buildable] func(BuildInitializer) (T, error)

func WrapBuildFactory[T Buildable](factory func(BuildInitializer) (T, error)) BuildFactoryTyped[T] {
	return buildFactoryWrapped[T](factory)
}

func (x buildFactoryWrapped[T]) Create(bi BuildInitializer) (Buildable, error) {
	return x(bi)
}
func (x buildFactoryWrapped[T]) Need(bi BuildInitializer, opts ...BuildOptionFunc) (T, error) {
	if buildable, err := bi.NeedFactory(x, opts...); err == nil {
		return buildable.(T), nil
	} else {
		var none T
		return none, err
	}
}
func (x buildFactoryWrapped[T]) SafeNeed(bi BuildInitializer, opts ...BuildOptionFunc) T {
	dst, err := x.Need(bi)
	LogPanicIfFailed(LogBuildGraph, err)
	return dst
}
func (x buildFactoryWrapped[T]) Output(bc BuildContext, opts ...BuildOptionFunc) (T, error) {
	if buildable, err := bc.OutputFactory(x, opts...); err == nil {
		return buildable.(T), nil
	} else {
		var none T
		return none, err
	}
}
func (x buildFactoryWrapped[T]) Init(bg BuildGraph, options ...BuildOptionFunc) (result T, err error) {
	var node *buildNode
	bo := NewBuildOptions(options...)
	node, err = InitBuildFactory(bg.(*buildGraph), x, &bo)
	if err == nil {
		result = node.Buildable.(T)
	}
	return
}
func (x buildFactoryWrapped[T]) Prepare(bg BuildGraph, options ...BuildOptionFunc) Future[T] {
	bo := NewBuildOptions(options...)
	future := PrepareBuildFactory(bg, x, &bo)
	return MapFuture(future, func(it BuildResult) (T, error) {
		return it.Buildable.(T), nil
	})
}
func (x buildFactoryWrapped[T]) Build(bg BuildGraph, options ...BuildOptionFunc) Result[T] {
	return x.Prepare(bg, options...).Join()
}

func InitBuildFactory(bg BuildGraph, factory BuildFactory, options *BuildOptions) (*buildNode, error) {
	return buildInit(bg.(*buildGraph), factory, options)
}
func PrepareBuildFactory(bg BuildGraph, factory BuildFactory, options *BuildOptions) Future[BuildResult] {
	node, err := InitBuildFactory(bg, factory, options)
	if err != nil {
		return MakeFutureError[BuildResult](err)
	}

	return bg.(*buildGraph).launchBuild(node, options)
}

/***************************************
 * Build Initializer
 ***************************************/

type buildInitializer struct {
	graph   *buildGraph
	options BuildOptions

	staticDeps BuildAliases
	sync.Mutex
}

func buildInit(g *buildGraph, factory BuildFactory, options *BuildOptions) (*buildNode, error) {
	context := buildInitializer{
		graph:      g,
		options:    options.Recurse(nil),
		staticDeps: BuildAliases{},
		Mutex:      sync.Mutex{},
	}

	buildable, err := factory.Create(&context)
	if err != nil {
		return nil, err
	}
	Assert(func() bool { return !IsNil(buildable) })

	node := g.Create(buildable, context.staticDeps, OptionBuildStruct(options))

	Assert(func() bool { return node.Alias().Equals(buildable.Alias()) })
	return node.(*buildNode), nil
}
func (x *buildInitializer) BuildGraph() BuildGraph {
	return x.graph
}
func (x *buildInitializer) Options() *BuildOptions {
	return &x.options
}
func (x *buildInitializer) DependsOn(aliases ...BuildAlias) error {
	x.Lock()
	defer x.Unlock()

	for _, alias := range aliases {
		if node := x.graph.Find(alias); node != nil {
			x.staticDeps.Append(alias)
		} else {
			return fmt.Errorf("static dependency not found: %q", alias)
		}
	}

	return nil
}
func (x *buildInitializer) NeedFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	bo := NewBuildOptions(OptionBuildStruct(&x.options))
	bo.Init(opts...)

	node, err := buildInit(x.graph, factory, &bo)
	if err != nil {
		return nil, err
	}

	x.Lock()
	defer x.Unlock()

	x.staticDeps.Append(node.Alias())
	return node.GetBuildable(), nil
}
func (x *buildInitializer) NeedFactories(factories ...BuildFactory) error {
	aliases := make(BuildAliases, len(factories))
	for i, factory := range factories {
		node, err := buildInit(x.graph, factory, &x.options)
		if err != nil {
			return err
		}
		aliases[i] = node.Alias()
	}

	x.Lock()
	defer x.Unlock()

	x.staticDeps.Append(aliases...)
	return nil
}
func (x *buildInitializer) NeedBuildable(buildables ...BuildAliasable) error {
	aliases := make([]BuildAlias, len(buildables))

	for i, buildable := range buildables {
		aliases[i] = buildable.Alias()

		if node := x.graph.Find(aliases[i]); node == nil {
			return fmt.Errorf("buildgraph: buildable %q not found", aliases[i])
		}
	}

	x.Lock()
	defer x.Unlock()

	x.staticDeps.Append(aliases...)
	return nil
}
func (x *buildInitializer) NeedFile(files ...Filename) error {
	for _, filename := range files {
		if _, err := x.NeedFactory(BuildFile(filename)); err != nil {
			return err
		}
	}
	return nil
}
func (x *buildInitializer) NeedDirectory(directories ...Directory) error {
	for _, directory := range directories {
		if _, err := x.NeedFactory(BuildDirectory(directory)); err != nil {
			return err
		}
	}
	return nil
}

/***************************************
 * Build Execute Context
 ***************************************/

type buildExecuteContext struct {
	graph   *buildGraph
	node    *buildNode
	options *BuildOptions

	previousStamp BuildStamp

	annotations StringSet
	stats       BuildStats
	timestamp   time.Time

	barrier sync.Mutex
}

type buildExecuteError struct {
	alias BuildAlias
	inner error
}

func (x buildExecuteError) Error() string {
	return fmt.Sprintf("node %q failed with: %v", x.alias, x.inner)
}

type buildDependencyError struct {
	alias BuildAlias
	link  BuildDependencyType
	inner error
}

func (x buildDependencyError) Error() string {
	return fmt.Sprintf("%s dependency of node %q failed with:\n\t%v", x.link, x.alias, x.inner)
}

func makeBuildExecuteContext(g *buildGraph, node *buildNode, options *BuildOptions) (result buildExecuteContext) {
	result = buildExecuteContext{
		graph:         g,
		node:          node,
		options:       options,
		timestamp:     CommandEnv.BuildTime(),
		previousStamp: node.GetBuildStamp(),
	}
	return
}

func (x *buildExecuteContext) prepareStaticDependencies_rlock() error {
	// need to call prebuild all static dependencies, since they could output this node
	bo := x.options.Recurse(x.node)
	future := x.graph.BuildMany(x.node.GetStaticDependencies(), OptionBuildStruct(&bo))
	if err := future.Join().Failure(); err != nil {
		return buildDependencyError{alias: x.Alias(), link: DEPENDENCY_STATIC, inner: err}
	}
	return nil
}

func (x *buildExecuteContext) buildOutputFiles_assumeLocked() Future[[]BuildResult] {
	results := make([]BuildResult, 0, len(x.node.OutputFiles))
	for _, it := range x.node.OutputFiles {
		node := x.graph.Find(it.Alias)
		if node == nil {
			return MakeFutureError[[]BuildResult](fmt.Errorf("build-graph: can't find buildable file %q", it.Alias))
		}

		file, ok := node.GetBuildable().(*Filename)
		AssertIn(ok, true)

		if stamp, err := file.Digest(); err == nil {
			results = append(results, BuildResult{
				Buildable:  file,
				BuildStamp: stamp,
			})
		} else {
			return MakeFutureError[[]BuildResult](err)
		}
	}
	return MakeFutureLiteral(results)
}
func (x *buildExecuteContext) needToBuild_assumeLocked() (bool, error) {
	if len(x.node.Static) == 0 && len(x.node.Dynamic) == 0 && len(x.node.OutputFiles) == 0 {
		return true, nil // nodes wihtout dependencies are systematically rebuilt
	}

	bo := x.options.Recurse(x.node)
	static := x.graph.BuildMany(x.node.Static.Aliases(), OptionBuildStruct(&bo))
	dynamic := x.graph.BuildMany(x.node.Dynamic.Aliases(), OptionBuildStruct(&bo))

	// output files are an exception: we can't build them without recursing into this node.
	// to avoid avoid looping (or more likely dead-locking) we don't build nodes for those,
	// but instead we compute directly the digest:
	outputFiles := x.buildOutputFiles_assumeLocked()
	// outputFiles := g.BuildMany(Keys(node.OutputFiles), bo, OptionBuildTouch(node))

	var lastError error
	rebuild := false

	if results, err := static.Join().Get(); err == nil {
		rebuild = rebuild || x.node.Static.updateBuild(x.node, DEPENDENCY_STATIC, results)
	} else {
		rebuild = false // wont't rebuild if a static dependency failed
		lastError = buildDependencyError{alias: x.Alias(), link: DEPENDENCY_STATIC, inner: err}
	}

	if results, err := dynamic.Join().Get(); err == nil {
		rebuild = rebuild || x.node.Dynamic.updateBuild(x.node, DEPENDENCY_DYNAMIC, results)
	} else {
		rebuild = false // wont't rebuild if a dynamic dependency failed
		lastError = buildDependencyError{alias: x.Alias(), link: DEPENDENCY_DYNAMIC, inner: err}
	}

	if results, err := outputFiles.Join().Get(); err == nil {
		rebuild = rebuild || x.node.OutputFiles.updateBuild(x.node, DEPENDENCY_OUTPUT, results)
	} else {
		rebuild = true // output dependencies can be regenerated
		if !x.options.NoWarningOnMissingOutput {
			LogWarning(LogBuildGraph, "%v: missing output, trigger rebuild -> %v", x.Alias(), err)
		}
	}

	// check if the node has a valid content fingerprint
	if !(rebuild || x.node.Stamp.Content.Valid()) {
		LogDebug(LogBuildGraph, "%v: invalid content fingerprint, trigger rebuild", x.Alias())
		// if not, then it needs to be rebuilt
		rebuild = true
	} else {
		if DEBUG_ENABLED && !rebuild {
			content := MakeBuildFingerprint(x.node.Buildable)
			AssertMessage(func() bool { return content == x.node.Stamp.Content }, "%v: content fingerprint does not match buildable:\n\tnode:      %v\n\tbuildable: %v", x.Alias(), x.node.Stamp.Content, content)
		}
	}

	return rebuild, lastError
}
func (x *buildExecuteContext) Execute() (BuildResult, bool, error) {
	x.stats = StartBuildStats()
	x.stats.pauseTimer()

	if err := x.prepareStaticDependencies_rlock(); err != nil {
		return BuildResult{
			Buildable:  x.node.GetBuildable(),
			BuildStamp: BuildStamp{},
		}, false, err
	}

	x.node.state.Lock()
	defer x.node.state.Unlock()

	needToBuild, err := x.needToBuild_assumeLocked()

	if err != nil {
		// make sure node will be built again
		x.node.Static.makeDirty()
		x.node.Dynamic.makeDirty()
		x.node.OutputFiles.makeDirty()

		return BuildResult{
			Buildable:  x.node.Buildable,
			BuildStamp: BuildStamp{},
		}, false, err
	}

	if !(needToBuild || x.options.Force) {
		return BuildResult{
			Buildable:  x.node.Buildable,
			BuildStamp: x.node.Stamp,
		}, false, nil
	}

	if needToBuild {
		// need to save the build graph since some dependency was invalidated (even if node own stamp did not change)
		x.graph.makeDirty()
	}

	defer func() {
		x.stats.resumeTimer()
		x.stats.stopTimer()

		Assert(func() bool { return x.node.Alias().Equals(x.node.Buildable.Alias()) })

		x.node.state.stats.add(&x.stats)
		x.graph.stats.atomic_add(&x.stats)
	}()

	// keep static dependencies untouched, clear everything else
	x.node.makeDirty_AssumeLocked()
	x.node.Stamp = BuildStamp{}

	Assert(func() bool { return x.node.Static.validate(x.node, DEPENDENCY_STATIC) })

	x.stats.resumeTimer()
	err = x.node.Buildable.Build(x)
	x.stats.pauseTimer()

	if err == nil {
		Assert(func() bool { return x.node.Dynamic.validate(x.node, DEPENDENCY_DYNAMIC) })
		Assert(func() bool { return x.node.OutputFiles.validate(x.node, DEPENDENCY_OUTPUT) })

		// update node timestamp when build succeeded
		x.node.Stamp = MakeTimedBuildFingerprint(x.timestamp, x.node.Buildable)
		Assert(func() bool { return x.node.Stamp.Content.Valid() })

		// need to save the build graph if build stamp changed
		if !needToBuild && x.previousStamp != x.node.Stamp {
			x.graph.makeDirty()
		}

		return BuildResult{
			Buildable:  x.node.Buildable,
			BuildStamp: x.node.Stamp,
		}, true, nil

	} else {
		Assert(func() bool { return !x.node.Stamp.Content.Valid() })

		// clear every dependency added by build
		x.node.makeDirty_AssumeLocked()
		// reset static timestamps to make sure this node is built again
		x.node.Static.makeDirty()

		err = buildExecuteError{alias: x.Alias(), inner: err}

		return BuildResult{
			Buildable:  x.node.Buildable,
			BuildStamp: BuildStamp{},
		}, true, err
	}
}
func (x *buildExecuteContext) OnBuilt(e func(BuildNode) error) {
	// add to parent to trigger the event in outer scope
	x.options.OnBuilt.Add(e)
}
func (x *buildExecuteContext) Alias() BuildAlias {
	return x.node.Alias()
}
func (x *buildExecuteContext) BuildGraph() BuildGraph {
	return x.graph
}
func (x *buildExecuteContext) Options() *BuildOptions {
	return x.options
}
func (x *buildExecuteContext) Annotate(text string) {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	x.annotations.Append(text)
}
func (x *buildExecuteContext) Timestamp(timestamp time.Time) {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	x.timestamp = timestamp
}
func (x *buildExecuteContext) lock_for_dependency() {
	x.barrier.Lock()
	x.stats.pauseTimer()
}
func (x *buildExecuteContext) unlock_for_dependency() {
	x.stats.resumeTimer()
	x.barrier.Unlock()
}
func (x *buildExecuteContext) dependsOn_AssumeLocked(aliases []BuildAlias, bo *BuildOptions) error {
	Assert(func() bool { return len(aliases) > 0 })

	result := x.graph.BuildMany(aliases, OptionBuildStruct(bo)).Join()
	if err := result.Failure(); err != nil {
		return err
	}

	for _, it := range result.Success() {
		x.node.addDynamic_AssumeLocked(it.Alias(), it.BuildStamp)
	}
	return nil
}
func (x *buildExecuteContext) DependsOn(aliases ...BuildAlias) error {
	if len(aliases) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false
	return x.dependsOn_AssumeLocked(aliases, &bo)
}
func (x *buildExecuteContext) NeedFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false
	bo.Init(opts...)

	node, err := buildInit(x.graph, factory, &bo)
	if err != nil {
		return nil, err
	}

	alias := node.Alias()
	_, future := x.graph.Build(node, OptionBuildStruct(&bo /* x.options */))

	if result := future.Join(); result.Failure() == nil {
		x.node.addDynamic_AssumeLocked(alias, result.Success().BuildStamp)
		return result.Success().Buildable, nil
	} else {
		return nil, result.Failure()
	}
}
func (x *buildExecuteContext) NeedFactories(factories ...BuildFactory) error {
	if len(factories) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false

	aliases := make(BuildAliases, len(factories))
	for i, factory := range factories {
		node, err := buildInit(x.graph, factory, &bo)
		if err != nil {
			return err
		}
		aliases[i] = node.Alias()
	}

	future := x.graph.BuildMany(aliases, OptionBuildStruct(&bo))
	return future.Join().Failure()
}
func (x *buildExecuteContext) NeedBuildable(buildables ...BuildAliasable) error {
	if len(buildables) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false
	return x.dependsOn_AssumeLocked(MakeBuildAliases(buildables...), &bo)
}
func (x *buildExecuteContext) NeedFile(files ...Filename) error {
	if len(files) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false

	aliases := make(BuildAliases, len(files))
	for i, file := range files {
		node, err := buildInit(x.graph, BuildFile(file), &bo)
		if err != nil {
			return err
		}
		aliases[i] = node.Alias()
	}

	return x.dependsOn_AssumeLocked(aliases, &bo)
}
func (x *buildExecuteContext) NeedDirectory(directories ...Directory) error {
	if len(directories) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false

	aliases := make(BuildAliases, len(directories))
	for i, directory := range directories {
		node, err := buildInit(x.graph, BuildDirectory(directory), &bo)
		if err != nil {
			return err
		}
		aliases[i] = node.Alias()
	}

	return x.dependsOn_AssumeLocked(aliases, &bo)
}
func (x *buildExecuteContext) OutputFile(files ...Filename) error {
	if len(files) == 0 {
		return nil
	}

	// files are treated as an exception: we build them outside of build scope, without using a future
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	// this code path always force recreates file nodes, which should be rebuild
	bo.Dirty = true
	bo.Force = true

	for _, it := range files {
		it = it.Normalize()

		LogDebug(LogBuildGraph, "%v: output file %q", x.Alias(), it)

		// create output file with a static dependency pointing to its creator (e.g x.node here)
		file, err := BuildFile(it, x.node.BuildAlias).Init(x.graph, OptionBuildStruct(&bo))
		if err != nil {
			return err
		}

		// output files are an exception: we need file build stamp to track external modifications
		// in the creator, but we also need to add a dependency from the creator on the file, creating a recursion.
		// to avoid looping (actually more dead-locking) we compute the build stamp of the file without
		// actually building its node:
		if stamp, err := file.Digest(); err == nil {
			x.node.addOutputFile_AssumeLocked(file.Alias(), stamp)
		} else {
			return err
		}
	}

	return nil
}
func (x *buildExecuteContext) outputFactory_AssumeLocked(factory BuildFactory, bo *BuildOptions) (Buildable, error) {
	creatorAlias := x.node.Alias()
	childFactory := WrapBuildFactory(func(bi BuildInitializer) (Buildable, error) {
		// add caller node as a static dependency
		if err := bi.DependsOn(creatorAlias); err != nil {
			return nil, err
		}
		return factory.Create(bi)
	})

	node, err := buildInit(x.graph, childFactory, bo)
	if err != nil {
		return nil, err
	}

	LogDebug(LogBuildGraph, "%v: outputs node %q", x.Alias(), node.Alias())
	x.node.addOutputNode_AssumeLocked(node.Alias())

	return node.Buildable, nil
}
func (x *buildExecuteContext) OutputNode(factories ...BuildFactory) error {
	if len(factories) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = true // this code path always force recreates the node

	for _, it := range factories {
		if _, err := x.outputFactory_AssumeLocked(it, &bo); err != nil {
			return err
		}
	}

	return nil
}
func (x *buildExecuteContext) OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	bo := x.options.Recurse(x.node)
	bo.Force = false
	bo.Init(opts...)

	if onBuilt := bo.OnBuilt; onBuilt.Bound() {
		x.OnBuilt(onBuilt.FireAndForget)
	}

	return x.outputFactory_AssumeLocked(factory, &bo)
}

/***************************************
 * Build Graph Context
 ***************************************/

type buildGraphContext struct {
	graph   *buildGraph
	options *BuildOptions
}

func (x buildGraphContext) BuildGraph() BuildGraph { return x.graph }
func (x buildGraphContext) Options() *BuildOptions { return x.options }

func (x buildGraphContext) DependsOn(aliases ...BuildAlias) error {
	bo := *x.options
	bo.Force = false
	future := x.graph.BuildMany(aliases, OptionBuildStruct(&bo))
	return future.Join().Failure()
}

func (x buildGraphContext) NeedFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	bo := *x.options
	bo.Force = false
	bo.Init(opts...)
	result, err := PrepareBuildFactory(x.graph, factory, &bo).Join().Get()
	if err == nil {
		return result.Buildable, nil
	} else {
		return nil, err
	}
}

func (x buildGraphContext) NeedFactories(factories ...BuildFactory) error {
	bo := *x.options
	bo.Force = false
	for _, future := range Map(func(factory BuildFactory) Future[BuildResult] {
		return PrepareBuildFactory(x.graph, factory, &bo)
	}, factories...) {
		if err := future.Join().Failure(); err != nil {
			return err
		}
	}
	return nil
}
func (x buildGraphContext) NeedBuildable(aliasables ...BuildAliasable) error {
	return x.DependsOn(MakeBuildAliases(aliasables...)...)
}
func (x buildGraphContext) NeedFile(filenames ...Filename) error {
	return x.NeedFactories(Map(func(f Filename) BuildFactory {
		return BuildFile(f)
	}, filenames...)...)
}
func (x buildGraphContext) NeedDirectory(dirnames ...Directory) error {
	return x.NeedFactories(Map(func(d Directory) BuildFactory {
		return BuildDirectory(d)
	}, dirnames...)...)
}

func (x buildGraphContext) OutputFile(filenames ...Filename) error {
	return x.OutputNode(Map(func(f Filename) BuildFactory {
		return BuildFile(f)
	}, filenames...)...)
}
func (x buildGraphContext) OutputNode(factories ...BuildFactory) error {
	bo := *x.options
	bo.Force = true
	for _, future := range Map(func(factory BuildFactory) Future[BuildResult] {
		return PrepareBuildFactory(x.graph, factory, &bo)
	}, factories...) {
		if err := future.Join().Failure(); err != nil {
			return err
		}
	}
	return nil
}

func (x buildGraphContext) OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	bo := *x.options
	bo.Force = true
	bo.Init(opts...)
	node, err := InitBuildFactory(x.graph, factory, &bo)
	if err != nil {
		return nil, err
	}

	return node.Buildable, nil
}

func (x *buildGraphContext) Annotate(s string) {
	LogVerbose(LogBuildGraph, "build annotate: %v", s)
}
func (x *buildGraphContext) Timestamp(t time.Time) {
	LogVerbose(LogBuildGraph, "build timestamp: %v", t)
}

func (x *buildGraphContext) OnBuilt(e func(BuildNode) error) {
	x.options.OnBuilt.Add(e)
}

/***************************************
 * Build Graph
 ***************************************/

type buildEvents struct {
	onBuildGraphStartEvent    ConcurrentEvent[BuildGraph]
	onBuildGraphFinishedEvent ConcurrentEvent[BuildGraph]

	onBuildNodeStartEvent    ConcurrentEvent[BuildNode]
	onBuildNodeFinishedEvent ConcurrentEvent[BuildNode]

	barrier         sync.Mutex
	pbar            ProgressScope
	numRunningTasks atomic.Int32
}

type buildGraph struct {
	flags   *CommandFlags
	nodes   *SharedStringMapT[*buildNode]
	options BuildOptions
	stats   BuildStats

	revision int32

	buildEvents
}

func NewBuildGraph(flags *CommandFlags, options ...BuildOptionFunc) BuildGraph {
	result := &buildGraph{
		flags:    flags,
		nodes:    NewSharedStringMap[*buildNode](128),
		options:  NewBuildOptions(options...),
		revision: 0,
	}
	return result
}

func (g *buildGraph) Aliases() []BuildAlias {
	keys := g.nodes.Keys()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	return Map(func(in string) BuildAlias { return BuildAlias(in) }, keys...)
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
		Assert(func() bool { return node.Alias().Equals(alias) })
		AssertMessage(func() bool { return node.Buildable.Alias().Equals(alias) }, "%v: node alias do not match buildable -> %q",
			alias, MakeStringer(func() string {
				return node.Buildable.Alias().String()
			}))

		LogTrace(LogBuildGraph, "Find(%q) -> %T", alias, node.Buildable)
		result = node
	} else {
		LogTrace(LogBuildGraph, "Find(%q) -> NOT_FOUND", alias)
	}

	return
}
func (g *buildGraph) Create(buildable Buildable, static BuildAliases, options ...BuildOptionFunc) BuildNode {
	bo := NewBuildOptions(options...)

	var node *buildNode
	var loaded bool

	alias := buildable.Alias()
	AssertMessage(func() bool { return alias.Valid() }, "invalid alias for <%T>", buildable)
	LogTrace(LogBuildGraph, "create <%T> node %q (force: %v, dirty: %v)", buildable, alias, bo.Force, bo.Dirty)

	if node, loaded = g.nodes.Get(alias.String()); loaded {
		// quick reject if a node with same alias already exists
		if !(bo.Force || bo.Dirty) {
			return node
		}
	} else {
		Assert(func() bool {
			makeCaseInsensitive := func(in string) string {
				return SanitizePath(strings.ToLower(in), '/')
			}
			lowerAlias := makeCaseInsensitive(alias.String())
			for _, key := range g.nodes.Keys() {
				lowerKey := makeCaseInsensitive(key)
				if lowerKey == lowerAlias {
					LogError(LogBuildGraph, "alias already registered with different case:\n\t add: %v\n\tfound: %v", alias, key)
					return false
				}
			}
			return true
		})

		// first optimistic Get() to avoid newBuildNode() bellow
		node, loaded = g.nodes.FindOrAdd(alias.String(), newBuildNode(alias, buildable))
	}

	defer LogPanicIfFailed(LogBuildGraph, bo.OnBuilt.Invoke(node))

	node.state.Lock()
	defer node.state.Unlock()

	Assert(func() bool { return alias == node.Alias() })
	AssertSameType(node.Buildable, buildable)

	LogPanicIfFailed(LogBuildGraph, bo.OnLaunched.Invoke(node))

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
		LogDebug(LogBuildGraph, "%v: dirty <%v> node depending on %v%v", alias,
			MakeStringer(func() string { return reflect.TypeOf(node.Buildable).String() }),
			node.Static.Aliases(),
			Blend("", " (forced update)", bo.Force))

		node.makeDirty_AssumeLocked()
		node.Static.makeDirty()

		g.makeDirty()
	}

	// node just went through a reset -> forget cached future if any
	node.future.Reset()

	AssertMessage(func() bool { return node.Buildable.Alias().Equals(alias) }, "%v: node alias do not match buildable -> %q",
		alias, MakeStringer(func() string {
			return node.Buildable.Alias().String()
		}))
	return node
}
func (g *buildGraph) Build(it BuildAliasable, options ...BuildOptionFunc) (BuildNode, Future[BuildResult]) {
	a := it.Alias()
	AssertMessage(func() bool { return a.Valid() }, "invalid alias for <%T>", it)

	if node, ok := g.nodes.Get(a.String()); ok {
		Assert(func() bool { return node.Alias().Equals(a) })
		AssertMessage(func() bool { return node.Buildable.Alias().Equals(a) }, "%v: node alias do not match buildable -> %q",
			a, MakeStringer(func() string {
				return node.Buildable.Alias().String()
			}))

		bo := NewBuildOptions(options...)
		return node, g.launchBuild(node, &bo)
	} else {
		return nil, MakeFutureError[BuildResult](fmt.Errorf("build: unknown node %q", a))
	}
}
func (g *buildGraph) BuildMany(targets BuildAliases, options ...BuildOptionFunc) (result Future[[]BuildResult]) {
	switch len(targets) {
	case 0:
		return MakeFutureLiteral([]BuildResult{})
	case 1:
		alias := targets[0]
		_, future := g.Build(alias, options...)

		return MapFuture(future, func(it BuildResult) ([]BuildResult, error) {
			return []BuildResult{it}, nil
		})
	default:
		return MakeFuture(func() (results []BuildResult, err error) {
			results = make([]BuildResult, len(targets))

			bo := NewBuildOptions(options...)
			boStruct := OptionBuildStruct(&bo)

			err = ParallelJoin(
				func(i int, it BuildResult) error {
					Assert(func() bool { return it.Content.Valid() })
					results[i] = it
					return nil
				},
				Map(func(alias BuildAlias) Future[BuildResult] {
					_, future := g.Build(alias, boStruct)
					return future
				}, targets...)...)

			return
		}, MakeStringer(func() string {
			return fmt.Sprintf("buildmany: %s", strings.Join(Stringize(targets...), ", "))
		}))
	}
}
func (g *buildGraph) Join() (lastErr error) {
	for lastErr == nil && g.numRunningTasks.Load() > 0 {
		g.nodes.Range(func(_ string, node *buildNode) {
			if node == nil {
				return
			}
			if future := node.future.Load(); future != nil {
				result := future.Join()
				if err := result.Failure(); err != nil {
					LogPanicErr(LogBuildGraph, err)
				}
			}
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
func (g *buildGraph) Serialize(ar Archive) {
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
	SerializeMany(ar, serialize, &pinned)
	if ar.Flags().IsLoading() && ar.Error() == nil {
		g.nodes.Clear()
		for _, node := range pinned {
			g.nodes.Add(node.Alias().String(), node)
		}
	}
}
func (g *buildGraph) Save(dst io.Writer) error {
	g.revision = 0
	return CompressedArchiveFileWrite(dst, g.Serialize)
}
func (g *buildGraph) Load(src io.Reader) error {
	g.revision = 0
	file, err := CompressedArchiveFileRead(src, g.Serialize)
	LogVeryVerbose(LogBuildGraph, "archive version = %v tags = %v", file.Version, file.Tags)
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

	g.nodes.Range(func(key string, node *buildNode) {
		if node.state.stats.Count != 0 {
			results = append(results, node)
			sort.Slice(results, predicate)

			if len(results) > n {
				results = results[:n]
			}
		}
	})
	return
}

func (g *buildGraph) makeDirty() {
	atomic.AddInt32(&g.revision, 1)
}
func (g *buildGraph) launchBuild(node *buildNode, options *BuildOptions) Future[BuildResult] {
	Assert(func() bool {
		relateOutp := strings.Builder{}
		if options.RelatesVerbose(node, 0, &relateOutp) {
			LogPanic(LogBuildGraph, "build cyclic dependency in %q\n%s", node, relateOutp.String())
			return false
		}
		return true
	})

	if BUILDGRAPH_ENABLE_CHECKS {
		relateOutp := strings.Builder{}
		if options.RelatesVerbose(node, 0, &relateOutp) {
			LogPanic(LogBuildGraph, "build cyclic dependency in %q\n%s", node, relateOutp.String())
		}
		LogTrace(LogBuildGraph, "buildgraph: launch build of <%T> %q\n%s", node.Buildable, node.Alias(), relateOutp.String())
	}

	var future = node.future.Load()
	if future != nil && !options.Force {
		return future
	}

	node.state.Lock()
	defer node.state.Unlock()

	if other := node.future.Load(); other != nil && other != future { // check if another thread already launched the node
		future = other
		return future
	} else {
		future = other
	}

	LogDebug(LogBuildGraph, "%v: launch <%v> build%v", node.Alias(),
		MakeStringer(func() string { return reflect.TypeOf(node.Buildable).String() }),
		MakeStringer(func() string { return Blend("", " (forced update)", options.Force) }))

	if future != nil {
		future.Join()
	}

	g.onBuildNodeStart(g, node)

	newFuture := MakeFuture(func() (BuildResult, error) {
		defer g.onBuildNodeFinished(g, node)

		context := makeBuildExecuteContext(g, node, options)
		result, built, err := context.Execute()

		if err == nil && built {
			err = options.OnBuilt.Invoke(node)
		}

		if err == nil {
			if built {
				changed := (result.BuildStamp != context.previousStamp)

				if changed {
					LogDebug(LogBuildGraph, "%v: new build stamp for [%T]\n\tnew: %v\n\told: %v", node.BuildAlias, result.Buildable, result.BuildStamp, context.previousStamp)
					g.makeDirty()
				}

				LogIf(LOG_INFO, LogBuildGraph, IsLogLevelActive(LOG_VERYVERBOSE) || !node.IsFile(),
					"%s%s %q in %v%s",
					Blend(``, `force `, options.Force),
					Blend(`build`, `update`, changed),
					node.BuildAlias,
					context.stats.Duration.Exclusive,
					MakeStringer(func() (annotations string) {
						if len(context.annotations) > 0 {
							annotations = fmt.Sprint(` (`, strings.Join(context.annotations, `, `), `)`)
						}
						return
					}))

			} else {
				LogVerbose(LogBuildGraph, "up-to-date %q%v",
					node.BuildAlias,
					Blend(``, `force `, options.Force))
			}

		} else {
			switch err.(type) {
			case buildDependencyError:
			default: // failed dependency errors are only printed once
				LogError(LogBuildGraph, "%v", err)
			}
		}

		return result, err
	})

	node.future.Store(newFuture)
	LogPanicIfFailed(LogBuildGraph, options.OnLaunched.Invoke(node))

	return newFuture
}

/***************************************
 * Build Events
 ***************************************/

func (g *buildEvents) onBuildGraphStart_ThreadSafe() ProgressScope {
	g.barrier.Lock()
	defer g.barrier.Unlock()

	if g.pbar == nil {
		g.pbar = LogSpinner("Build Graph ")
	}
	return g.pbar
}
func (g *buildEvents) onBuildGraphFinished_ThreadSafe() {
	g.barrier.Lock()
	defer g.barrier.Unlock()

	if g.pbar != nil {
		g.pbar.Close()
		g.pbar = nil
	}
}

func (g *buildEvents) onBuildNodeStart(graph *buildGraph, node *buildNode) {
	if g.numRunningTasks.Add(1) == 1 {
		g.onBuildGraphStartEvent.Invoke(graph)
	}

	g.onBuildNodeStartEvent.Invoke(node)

	if enableInteractiveShell {
		g.onBuildGraphStart_ThreadSafe()

		if g.pbar != nil {
			g.pbar.Grow(1)
			g.pbar.Log("Built %d / %d nodes", g.pbar.Progress(), g.pbar.Len())
		}
	}
}
func (g *buildEvents) onBuildNodeFinished(graph *buildGraph, node *buildNode) {
	g.onBuildNodeFinishedEvent.Invoke(node)

	if g.numRunningTasks.Add(-1) == 0 {
		g.onBuildGraphFinishedEvent.Invoke(graph)
		g.onBuildGraphFinished_ThreadSafe()
	}

	if enableInteractiveShell && g.pbar != nil {
		g.pbar.Inc()
		g.pbar.Log("Built %d / %d nodes", g.pbar.Progress(), g.pbar.Len())
	}
}

func (g *buildEvents) OnBuildGraphStart() MutableEvent[BuildGraph] {
	return &g.onBuildGraphStartEvent
}
func (g *buildEvents) OnBuildGraphFinished() MutableEvent[BuildGraph] {
	return &g.onBuildGraphFinishedEvent
}

func (g *buildEvents) OnBuildNodeStart() MutableEvent[BuildNode] {
	return &g.onBuildNodeStartEvent
}
func (g *buildEvents) OnBuildNodeFinished() MutableEvent[BuildNode] {
	return &g.onBuildNodeStartEvent
}

/***************************************
 * Build Alias
 ***************************************/

func MakeBuildAliases[T BuildAliasable](targets ...T) (result BuildAliases) {
	result = make(BuildAliases, len(targets))

	for i, it := range targets {
		result[i] = it.Alias()
	}

	return result
}

func ConcatBuildAliases[T BuildAliasable](targets ...[]T) (result BuildAliases) {
	capacity := 0
	for _, arr := range targets {
		capacity += len(arr)
	}

	result = make(BuildAliases, capacity)

	i := 0
	for _, arr := range targets {
		for _, it := range arr {
			result[i] = it.Alias()
			i++
		}
	}

	return result
}

func FindBuildAliases(bg BuildGraph, category string, names ...string) (result BuildAliases) {
	prefix := MakeBuildAlias(category, names...).String()
	for _, a := range bg.Aliases() {
		if strings.HasPrefix(a.String(), prefix) {
			result.Append(a)
		}
	}
	return
}

func MakeBuildAlias(category string, names ...string) BuildAlias {
	sb := strings.Builder{}
	sep := "://"

	capacity := len(category)
	i := 0
	for _, it := range names {
		if len(it) == 0 {
			continue
		}
		if i > 0 {
			capacity++
		} else {
			capacity += len(sep)
		}
		capacity += len(it)
		i++
	}
	sb.Grow(capacity)

	sb.WriteString(category)
	i = 0
	for _, it := range names {
		if len(it) == 0 {
			continue
		}
		if i > 0 {
			sb.WriteRune('/')
		} else {
			sb.WriteString(sep)
		}
		BuildSanitizedPath(&sb, it, '/')
		i++
	}

	return BuildAlias(sb.String())
}
func (x BuildAlias) Alias() BuildAlias { return x }
func (x BuildAlias) Valid() bool       { return len(x) > 3 /* check for "---" */ }
func (x BuildAlias) Equals(o BuildAlias) bool {
	return (string)(x) == (string)(o)
}
func (x BuildAlias) Compare(o BuildAlias) int {
	return strings.Compare((string)(x), (string)(o))
}
func (x BuildAlias) String() string {
	Assert(func() bool { return x.Valid() })
	return (string)(x)
}
func (x *BuildAlias) Set(in string) error {
	Assert(func() bool { return x.Valid() })
	*x = BuildAlias(in)
	return nil
}
func (x *BuildAlias) Serialize(ar Archive) {
	ar.String((*string)(x))
}
func (x *BuildAlias) MarshalText() ([]byte, error) {
	return UnsafeBytesFromString(x.String()), nil
}
func (x *BuildAlias) UnmarshalText(data []byte) error {
	return x.Set(UnsafeStringFromBytes(data))
}

/***************************************
 * Build Stamp
 ***************************************/

func MakeBuildFingerprint(buildable Buildable) (result Fingerprint) {
	result = SerializeFingerpint(buildable, GetProcessSeed())
	if !result.Valid() {
		LogPanic(LogBuildGraph, "buildgraph: invalid buildstamp for %q", buildable.Alias())
	}
	return
}
func MakeTimedBuildStamp(modTime time.Time, fingerprint Fingerprint) BuildStamp {
	return BuildStamp{
		// round up timestamp to millisecond, see ArchiveBinaryReader/Writer.Time()
		ModTime: time.UnixMilli(modTime.UnixMilli()),
		Content: fingerprint,
	}
}
func MakeTimedBuildFingerprint(modTime time.Time, buildable Buildable) (result BuildStamp) {
	result = MakeTimedBuildStamp(modTime, MakeBuildFingerprint(buildable))
	LogTrace(LogBuildGraph, "MakeTimedBuildFingerprint(%v, %q) -> %v", modTime, buildable.Alias(), result)
	return
}

func (x BuildStamp) String() string {
	return fmt.Sprintf("[%v] %v", x.Content.ShortString(), x.ModTime.Local().Format(time.Stamp))
}
func (x *BuildStamp) Serialize(ar Archive) {
	ar.Time(&x.ModTime)
	ar.Serializable(&x.Content)
}

/***************************************
 * Build Options
 ***************************************/

func NewBuildOptions(options ...BuildOptionFunc) (result BuildOptions) {
	result.Init(options...)
	return
}
func (x *BuildOptions) Init(options ...BuildOptionFunc) {
	for _, it := range options {
		it(x)
	}
}
func (x *BuildOptions) Recurse(node BuildNode) (result BuildOptions) {
	AssertMessage(func() bool { return x.Caller == nil || x.Caller != node }, "build graph: invalid build recursion on %q\n%v", node, x)
	AssertMessage(func() bool { return node == nil || node.Alias().Valid() }, "build graph: invalid build alias on %q\n%v", node, x)

	result.Parent = x
	result.Caller = node
	result.NoWarningOnMissingOutput = x.NoWarningOnMissingOutput

	if x.Recursive {
		result.Force = x.Force
		result.Recursive = x.Recursive
	}

	return
}
func (x *BuildOptions) Touch(parent BuildNode) (result BuildOptions) {
	Assert(func() bool { return x.Caller == parent })
	x.Stamp = &x.Caller.(*buildNode).Stamp
	return
}
func (x BuildOptions) DependencyChain() (result []BuildNode) {
	result = []BuildNode{x.Caller}
	if x.Parent != nil {
		result = append(result, x.Parent.DependencyChain()...)
	}
	return
}
func (x BuildOptions) String() string {
	sb := strings.Builder{}
	x.RelatesVerbose(nil, 0, &sb)
	return sb.String()
}
func (x BuildOptions) HasOuter(node BuildNode) *BuildStamp {
	if x.Caller == node {
		return x.Stamp
	} else if x.Parent != nil {
		return x.Parent.HasOuter(node)
	}
	return nil
}
func (x BuildOptions) RelatesVerbose(node BuildNode, depth int, outp *strings.Builder) bool {
	indent := "  "
	if depth == 0 && node != nil {
		fmt.Fprintf(outp, "%s%d) %s\n", strings.Repeat(indent, depth), depth, node.Alias())
		depth++
	}
	if x.Caller != nil {
		if x.Stamp == nil {
			fmt.Fprintf(outp, "%s%d) %s\n", strings.Repeat(indent, depth), depth, x.Caller.Alias())
		} else {
			fmt.Fprintf(outp, "%s%d) %s - [OUTER:%s]\n", strings.Repeat(indent, depth), depth, x.Caller.Alias(), x.Stamp.String())
		}
	}
	if depth > 20 {
		LogPanic(LogBuildGraph, "buildgraph: node stack too deep!\n%v", outp)
	}

	var result bool
	if x.Caller == node && x.Stamp == nil /* if Caller has a Stamp then it is the outer of 'node' */ {
		result = true
	} else if x.Parent != nil {
		result = x.Parent.RelatesVerbose(node, depth+1, outp)
	} else {
		result = false
	}

	return result
}

func OptionBuildCaller(node BuildNode) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.Caller = node
	}
}
func OptionBuildTouch(node BuildNode) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.Touch(node)
	}
}
func OptionBuildDirty(opts *BuildOptions) {
	opts.Dirty = true
}
func OptionBuildDirtyIf(dirty bool) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.Dirty = dirty
	}
}
func OptionBuildForce(opts *BuildOptions) {
	opts.Force = true
}
func OptionBuildForceIf(force bool) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.Force = force
	}
}
func OptionBuildForceRecursive(opts *BuildOptions) {
	opts.Force = true
	opts.Recursive = true
}
func OptionNoWarningOnMissingOutput(opts *BuildOptions) {
	opts.NoWarningOnMissingOutput = true
}
func OptionWarningOnMissingOutputIf(warn bool) BuildOptionFunc {
	return func(bo *BuildOptions) {
		bo.NoWarningOnMissingOutput = !warn
	}
}
func OptionBuildOnLaunched(event func(BuildNode) error) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.OnLaunched.Add(event)
	}
}
func OptionBuildOnBuilt(event func(BuildNode) error) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.OnBuilt.Add(event)
	}
}
func OptionBuildParent(parent *BuildOptions) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.Parent = parent
	}
}
func OptionBuildStruct(value *BuildOptions) BuildOptionFunc {
	return func(opts *BuildOptions) {
		*opts = *value
	}
}

/***************************************
 * Build Dependencies
 ***************************************/

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
		UnexpectedValuePanic(x, x)
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
		return MakeUnexpectedValueError(x, in)
	}
	return nil
}

func (x *BuildDependency) Serialize(ar Archive) {
	ar.Serializable(&x.Alias)
	ar.Serializable(&x.Stamp)
}

func (deps BuildDependencies) Aliases() BuildAliases {
	result := make(BuildAliases, len(deps))
	for i, it := range deps {
		result[i] = it.Alias
	}
	return result
}
func (deps BuildDependencies) Copy() BuildDependencies {
	return CopySlice(deps...)
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
func (deps *BuildDependencies) Serialize(ar Archive) {
	SerializeSlice(ar, (*[]BuildDependency)(deps))
}
func (deps BuildDependencies) validate(owner BuildNode, depType BuildDependencyType) bool {
	valid := true
	for _, it := range deps {
		if !it.Stamp.Content.Valid() {
			valid = false
			LogError(LogBuildGraph, "%v: %s dependency <%v> has an invalid build stamp (%v)", depType, owner.Alias(), it.Alias, it.Stamp)
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
	Assert(func() bool { return len(results) == len(*deps) })

	for _, result := range results {
		alias := result.Alias()
		Assert(func() bool { return result.Content.Valid() })

		oldStampIndex, ok := deps.IndexOf(alias)
		AssertIn(ok, true)
		oldStamp := (*deps)[oldStampIndex].Stamp

		if oldStamp != result.BuildStamp {
			LogTrace(LogBuildGraph, "%v: %v dependency <%v> has been updated:\n\tnew: %v\n\told: %v", owner.Alias(), depType, alias, result.BuildStamp, oldStamp)

			deps.Add(alias, result.BuildStamp)
			rebuild = true
		} // else // LogDebug("%v: %v %v dependency is up-to-date", owner.Alias(), alias, depType)
	}

	return rebuild
}

/***************************************
 * Build Stats
 ***************************************/

func StartBuildStats() (result BuildStats) {
	result.startTimer()
	return
}
func (x *BuildStats) Append(other *BuildStats) {
	other.stopTimer()
	x.atomic_add(other)
}

func (x *BuildStats) atomic_add(other *BuildStats) {
	if atomic.AddInt32(&x.Count, other.Count) == other.Count {
		x.InclusiveStart = other.InclusiveStart
		x.ExclusiveStart = other.ExclusiveStart
	}

	atomic.AddInt64((*int64)(&x.Duration.Inclusive), int64(other.Duration.Inclusive))
	atomic.AddInt64((*int64)(&x.Duration.Exclusive), int64(other.Duration.Exclusive))
}
func (x *BuildStats) add(other *BuildStats) {
	if x.Count == 0 {
		x.InclusiveStart = other.InclusiveStart
		x.ExclusiveStart = other.ExclusiveStart
	}

	x.Count += other.Count
	x.Duration.Inclusive += other.Duration.Inclusive
	x.Duration.Exclusive += other.Duration.Exclusive
}
func (x *BuildStats) startTimer() {
	x.Count++
	x.InclusiveStart = Elapsed()
	x.ExclusiveStart = x.InclusiveStart
}
func (x *BuildStats) stopTimer() {
	elapsed := Elapsed()
	x.Duration.Inclusive += (elapsed - x.InclusiveStart)
	x.Duration.Exclusive += (elapsed - x.ExclusiveStart)
}
func (x *BuildStats) pauseTimer() {
	x.Duration.Exclusive += (Elapsed() - x.ExclusiveStart)
}
func (x *BuildStats) resumeTimer() {
	x.ExclusiveStart = Elapsed()
}
