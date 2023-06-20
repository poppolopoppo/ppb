package utils

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/poppolopoppo/ppb/internal/base"
)

type BuildOptions struct {
	Parent                   *BuildOptions
	Caller                   BuildNode
	OnLaunched               base.PublicEvent[BuildNode]
	OnBuilt                  base.PublicEvent[BuildNode]
	Stamp                    *BuildStamp
	Dirty                    bool
	Force                    bool
	Recursive                bool
	NoWarningOnMissingOutput bool
}

type BuildOptionFunc func(*BuildOptions)

type BuildContext interface {
	BuildInitializer

	OutputFile(...Filename) error
	OutputNode(...BuildFactory) error

	OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error)

	Annotate(string)
	Timestamp(time.Time)

	OnBuilt(func(BuildNode) error)
}

/***************************************
 * Build Execute Context
 ***************************************/

type buildExecuteContext struct {
	graph   *buildGraph
	node    *buildNode
	options *BuildOptions

	previousStamp BuildStamp

	annotations base.StringSet
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

func (x *buildExecuteContext) buildOutputFiles_assumeLocked() base.Future[[]BuildResult] {
	results := make([]BuildResult, 0, len(x.node.OutputFiles))
	for _, it := range x.node.OutputFiles {
		node := x.graph.Find(it.Alias)
		if node == nil {
			return base.MakeFutureError[[]BuildResult](fmt.Errorf("build-graph: can't find buildable file %q", it.Alias))
		}

		file, ok := node.GetBuildable().(*Filename)
		base.AssertIn(ok, true)

		if stamp, err := file.Digest(); err == nil {
			results = append(results, BuildResult{
				Buildable:  file,
				BuildStamp: stamp,
			})
		} else {
			return base.MakeFutureError[[]BuildResult](err)
		}
	}
	return base.MakeFutureLiteral(results)
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
			base.LogWarning(LogBuildGraph, "%v: missing output, trigger rebuild -> %v", x.Alias(), err)
		}
	}

	// check if the node has a valid content fingerprint
	if !(rebuild || x.node.Stamp.Content.Valid()) {
		base.LogDebug(LogBuildGraph, "%v: invalid content fingerprint, trigger rebuild", x.Alias())
		// if not, then it needs to be rebuilt
		rebuild = true
	} else {
		if base.DEBUG_ENABLED && !rebuild {
			content := MakeBuildFingerprint(x.node.Buildable)
			base.AssertErr(func() error {
				if content == x.node.Stamp.Content {
					return nil
				}
				return fmt.Errorf("%v: content fingerprint does not match buildable:\n\tnode:      %v\n\tbuildable: %v", x.Alias(), x.node.Stamp.Content, content)
			})
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

	x.graph.onBuildNodeStart_ThreadSafe(x.graph, x.node)
	defer x.graph.onBuildNodeFinished_ThreadSafe(x.graph, x.node) // see launchBuild() for onBuildNodeStart_ThreadSafe()

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

		base.Assert(func() bool { return x.node.Alias().Equals(x.node.Buildable.Alias()) })

		x.node.state.stats.add(&x.stats)
		x.graph.stats.atomic_add(&x.stats)
	}()

	// keep static dependencies untouched, clear everything else
	x.node.makeDirty_AssumeLocked()
	x.node.Stamp = BuildStamp{}

	base.Assert(func() bool { return x.node.Static.validate(x.node, DEPENDENCY_STATIC) })

	x.stats.resumeTimer()
	err = x.node.Buildable.Build(x)
	x.stats.pauseTimer()

	if err == nil {
		base.Assert(func() bool { return x.node.Dynamic.validate(x.node, DEPENDENCY_DYNAMIC) })
		base.Assert(func() bool { return x.node.OutputFiles.validate(x.node, DEPENDENCY_OUTPUT) })

		// update node timestamp when build succeeded
		x.node.Stamp = MakeTimedBuildFingerprint(x.timestamp, x.node.Buildable)
		base.Assert(func() bool { return x.node.Stamp.Content.Valid() })

		// need to save the build graph if build stamp changed
		if !needToBuild && x.previousStamp != x.node.Stamp {
			x.graph.makeDirty()
		}

		return BuildResult{
			Buildable:  x.node.Buildable,
			BuildStamp: x.node.Stamp,
		}, true, nil

	} else {
		base.Assert(func() bool { return !x.node.Stamp.Content.Valid() })

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

func (x *buildExecuteContext) OnBuilt(e func(BuildNode) error) {
	// add to parent to trigger the event in outer scope
	x.options.OnBuilt.Add(e)
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
	base.Assert(func() bool { return len(aliases) > 0 })

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

		base.LogDebug(LogBuildGraph, "%v: output file %q", x.Alias(), it)

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

	base.LogDebug(LogBuildGraph, "%v: outputs node %q", x.Alias(), node.Alias())
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
	for _, future := range base.Map(func(factory BuildFactory) base.Future[BuildResult] {
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
	return x.NeedFactories(base.Map(func(f Filename) BuildFactory {
		return BuildFile(f)
	}, filenames...)...)
}
func (x buildGraphContext) NeedDirectory(dirnames ...Directory) error {
	return x.NeedFactories(base.Map(func(d Directory) BuildFactory {
		return BuildDirectory(d)
	}, dirnames...)...)
}

func (x buildGraphContext) OutputFile(filenames ...Filename) error {
	return x.OutputNode(base.Map(func(f Filename) BuildFactory {
		return BuildFile(f)
	}, filenames...)...)
}
func (x buildGraphContext) OutputNode(factories ...BuildFactory) error {
	bo := *x.options
	bo.Force = true
	for _, future := range base.Map(func(factory BuildFactory) base.Future[BuildResult] {
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
	base.LogVerbose(LogBuildGraph, "build annotate: %v", s)
}
func (x *buildGraphContext) Timestamp(t time.Time) {
	base.LogVerbose(LogBuildGraph, "build timestamp: %v", t)
}

func (x *buildGraphContext) OnBuilt(e func(BuildNode) error) {
	x.options.OnBuilt.Add(e)
}

/***************************************
 * Build Stamp
 ***************************************/

func MakeBuildFingerprint(buildable Buildable) (result base.Fingerprint) {
	result = base.SerializeFingerpint(buildable, GetProcessSeed())
	if !result.Valid() {
		base.LogPanic(LogBuildGraph, "buildgraph: invalid buildstamp for %q", buildable.Alias())
	}
	return
}
func MakeTimedBuildStamp(modTime time.Time, fingerprint base.Fingerprint) BuildStamp {
	return BuildStamp{
		// round up timestamp to millisecond, see ArchiveBinaryReader/Writer.Time()
		ModTime: time.UnixMilli(modTime.UnixMilli()),
		Content: fingerprint,
	}
}
func MakeTimedBuildFingerprint(modTime time.Time, buildable Buildable) (result BuildStamp) {
	result = MakeTimedBuildStamp(modTime, MakeBuildFingerprint(buildable))
	base.LogTrace(LogBuildGraph, "MakeTimedBuildFingerprint(%v, %q) -> %v", modTime, buildable.Alias(), result)
	return
}

func (x BuildStamp) String() string {
	return fmt.Sprintf("[%v] %v", x.Content.ShortString(), x.ModTime.Local().Format(time.Stamp))
}
func (x *BuildStamp) Serialize(ar base.Archive) {
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
	base.AssertErr(func() error {
		if x.Caller == nil || x.Caller != node {
			return nil
		}
		return fmt.Errorf("build graph: invalid build recursion on %q\n%v", node, x)
	})
	base.AssertErr(func() error {
		if node == nil || node.Alias().Valid() {
			return nil
		}
		return fmt.Errorf("build graph: invalid build alias on %q\n%v", node, x)
	})

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
	base.Assert(func() bool { return x.Caller == parent })
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
		base.LogPanic(LogBuildGraph, "buildgraph: node stack too deep!\n%v", outp)
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
 * Launch build for a node
 ***************************************/

func (g *buildGraph) launchBuild(node *buildNode, options *BuildOptions) base.Future[BuildResult] {
	base.Assert(func() bool {
		relateOutp := strings.Builder{}
		if options.RelatesVerbose(node, 0, &relateOutp) {
			base.LogPanic(LogBuildGraph, "build cyclic dependency in %q\n%s", node, relateOutp.String())
			return false
		}
		return true
	})

	if BUILDGRAPH_ENABLE_CHECKS {
		relateOutp := strings.Builder{}
		if options.RelatesVerbose(node, 0, &relateOutp) {
			base.LogPanic(LogBuildGraph, "build cyclic dependency in %q\n%s", node, relateOutp.String())
		}
		base.LogTrace(LogBuildGraph, "buildgraph: launch build of <%T> %q\n%s", node.Buildable, node.Alias(), relateOutp.String())
	}

	var future = node.future.Load()
	if future != nil {
		if !options.Force {
			return future
		}
	}

	node.state.Lock()
	defer node.state.Unlock()

	if other := node.future.Load(); other != nil && other != future { // check if another thread already launched the node
		future = other
		return future
	} else {
		future = other
	}

	if future != nil {
		future.Join()
	}

	newFuture := base.MakeFuture(func() (BuildResult, error) {
		context := makeBuildExecuteContext(g, node, options)
		result, built, err := context.Execute()

		if err == nil && built {
			err = options.OnBuilt.Invoke(node)
		}

		if err == nil {
			if built {
				changed := (result.BuildStamp != context.previousStamp)

				if changed {
					base.LogDebug(LogBuildGraph, "%v: new build stamp for [%T]\n\tnew: %v\n\told: %v", node.BuildAlias, result.Buildable, result.BuildStamp, context.previousStamp)
					g.makeDirty()
				}

				base.LogIf(base.LOG_INFO, LogBuildGraph, base.IsLogLevelActive(base.LOG_VERYVERBOSE) || !node.IsFile(),
					"%s%s %q in %v%s",
					base.Blend(``, `force `, options.Force),
					base.Blend(`build`, `update`, changed),
					node.BuildAlias,
					context.stats.Duration.Exclusive,
					base.MakeStringer(func() (annotations string) {
						if len(context.annotations) > 0 {
							annotations = fmt.Sprint(` (`, strings.Join(context.annotations, `, `), `)`)
						}
						return
					}))

			} else {
				base.LogVerbose(LogBuildGraph, "up-to-date %q%v",
					node.BuildAlias,
					base.Blend(``, `force `, options.Force))
			}

		} else {
			switch err.(type) {
			case buildDependencyError:
			default: // failed dependency errors are only printed once
				base.LogError(LogBuildGraph, "%v", err)
			}
		}

		return result, err
	})

	node.future.Store(newFuture)
	base.LogPanicIfFailed(LogBuildGraph, options.OnLaunched.Invoke(node))

	return newFuture
}
