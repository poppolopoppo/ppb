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

type BuildAnnotations struct {
	Comments  []string
	Dirty     bool
	Mute      bool
	Timestamp time.Time
}

type BuildAnnotateFunc func(*BuildAnnotations)

type BuildContext interface {
	BuildInitializer

	CheckForAbort() error

	GetStaticDependencyBuildResults() []BuildResult

	NeedBuildAliasables(n int, buildAliasables func(int) BuildAliasable, onBuildResult func(int, BuildResult) error) error
	NeedBuildResult(...BuildResult)

	OutputFile(...Filename) error
	OutputNode(...BuildFactory) error

	OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error)

	Annotate(...BuildAnnotateFunc)

	OnBuilt(func(BuildNode) error)
}

/***************************************
 * Build Execute Context
 ***************************************/

type buildExecuteContext struct {
	*buildGraphWritePort

	node    *buildNode
	options *BuildOptions

	previousStamp BuildStamp

	annotations   BuildAnnotations
	staticResults []BuildResult
	stats         BuildStats

	barrier sync.Mutex
}

type buildAbortError struct {
	inner error
}

func (x buildAbortError) Error() string {
	return "build aborted"
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

func makeBuildExecuteContext(g *buildGraphWritePort, node *buildNode, options *BuildOptions) (result buildExecuteContext) {
	result = buildExecuteContext{
		buildGraphWritePort: g,
		node:                node,
		options:             options,
		annotations: BuildAnnotations{
			Timestamp: CommandEnv.BuildTime(),
		},
		previousStamp: node.GetBuildStamp(),
	}
	return
}

func (x *buildExecuteContext) prepareStaticDependencies_rlock() error {
	// need to call prebuild all static dependencies, since they could output this node
	staticDeps := x.node.GetStaticDependencies()

	// record static dependency results for clients
	x.staticResults = make([]BuildResult, len(staticDeps))

	if err := x.buildMany(len(staticDeps),
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(staticDeps[i])
		},
		func(i int, br BuildResult) error {
			x.staticResults[i] = br
			return nil
		},
		OptionBuildRecurse(x.options, x.node)); err != nil {
		return buildDependencyError{alias: x.Alias(), link: DEPENDENCY_STATIC, inner: err}
	}
	return nil
}

func (x *buildExecuteContext) buildOutputFiles_assumeLocked() base.Future[[]BuildResult] {
	results := make([]BuildResult, 0, len(x.node.OutputFiles))
	for _, it := range x.node.OutputFiles {
		node, err := x.Expect(it.Alias)
		if err != nil {
			return base.MakeFutureError[[]BuildResult](err)
		}

		file, ok := node.GetBuildable().(*FileDependency)
		base.AssertIn(ok, true)

		fileStamp, err := buildFileStampWithoutDeps(file.Filename)
		if err != nil {
			return base.MakeFutureError[[]BuildResult](err)
		}

		results = append(results, BuildResult{
			BuildAlias: it.Alias,
			Buildable:  file,
			BuildStamp: fileStamp,
		})
	}
	return base.MakeFutureLiteral(results)
}
func (x *buildExecuteContext) needToBuild_assumeLocked() (bool, error) {
	if len(x.node.Static) == 0 && len(x.node.Dynamic) == 0 && len(x.node.OutputFiles) == 0 {
		return true, nil // nodes wihtout dependencies are systematically rebuilt
	}

	static := x.launchBuildMany(len(x.node.Static),
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(x.node.Static[i].Alias)
		}, OptionBuildRecurse(x.options, x.node))
	dynamic := x.launchBuildMany(len(x.node.Dynamic),
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(x.node.Dynamic[i].Alias)
		}, OptionBuildRecurse(x.options, x.node))

	// output files are an exception: we can't build them without recursing into this node.
	// to avoid avoid looping (or more likely dead-locking) we don't build nodes for those,
	// but instead we compute directly the digest:
	outputFiles := x.buildOutputFiles_assumeLocked()
	// outputFiles := g.BuildMany(Keys(node.OutputFiles), bo, OptionBuildTouch(node))

	var lastError error
	rebuild := false

	// check if a static dependency was updated
	if results, err := static.Join().Get(); err == nil {
		rebuild = rebuild || x.node.Static.updateBuild(x.node, DEPENDENCY_STATIC, results)
	} else {
		rebuild = false // wont't rebuild if a static dependency failed
		lastError = buildDependencyError{alias: x.Alias(), link: DEPENDENCY_STATIC, inner: err}
	}

	// check if a dynamic was updated
	if results, err := dynamic.Join().Get(); err == nil {
		rebuild = rebuild || x.node.Dynamic.updateBuild(x.node, DEPENDENCY_DYNAMIC, results)
	} else if !rebuild {
		// wont't rebuild if a dynamic dependency failed and static are oks
		lastError = buildDependencyError{alias: x.Alias(), link: DEPENDENCY_DYNAMIC, inner: err}
	}

	// check if an output file was updated
	if results, err := outputFiles.Join().Get(); err == nil {
		rebuild = rebuild || x.node.OutputFiles.updateBuild(x.node, DEPENDENCY_OUTPUT, results)
	} else {
		rebuild = true // output dependencies can be regenerated
		if !x.options.NoWarningOnMissingOutput {
			base.LogWarning(LogBuildGraph, "%v: missing output, trigger rebuild -> %v", x.Alias(), err)
		}
	}

	// graph needs to be resaved if any dependency was updated
	if rebuild {
		x.makeDirty("dependency updated")
	}

	// check if the node has a valid content fingerprint
	if !rebuild && !x.node.Stamp.Content.Valid() {
		rebuild = true
		base.LogDebug(LogBuildGraph, "%v: invalid content fingerprint, trigger rebuild", x.Alias())
	}

	base.AssertErr(func() error {
		if !rebuild {
			content := MakeBuildFingerprint(x.node.Buildable)
			if content != x.node.Stamp.Content {
				return fmt.Errorf("%v: content fingerprint does not match buildable:\n\tnode:      %v\n\tbuildable: %v", x.Alias(), x.node.Stamp.Content, content)
			}
		}
		return nil
	})

	return rebuild, lastError
}
func (x *buildExecuteContext) Execute(state *buildState) (BuildResult, bool, error) {
	x.stats = StartBuildStats()
	x.stats.pauseTimer()

	if err := x.prepareStaticDependencies_rlock(); err != nil {
		return BuildResult{
			BuildAlias: x.node.BuildAlias,
			Buildable:  x.node.GetBuildable(),
			BuildStamp: BuildStamp{},
		}, false, err
	}

	x.node.Lock()
	defer x.node.Unlock()

	needToBuild, err := x.needToBuild_assumeLocked()

	if err != nil {
		// make sure node will be built again
		x.node.Static.makeDirty()
		x.node.Dynamic.makeDirty()
		x.node.OutputFiles.makeDirty()

		return BuildResult{
			BuildAlias: x.node.BuildAlias,
			Buildable:  x.node.Buildable,
			BuildStamp: BuildStamp{},
		}, false, err
	}

	if !(needToBuild || x.options.Force) {
		return BuildResult{
			BuildAlias: x.node.BuildAlias,
			Buildable:  x.node.Buildable,
			BuildStamp: x.node.Stamp,
		}, false, nil
	}

	if err := x.CheckForAbort(); err != nil {
		return BuildResult{
			BuildAlias: x.node.BuildAlias,
			Buildable:  x.node.GetBuildable(),
			BuildStamp: BuildStamp{},
		}, false, err
	}

	defer func() {
		x.stats.resumeTimer()
		x.stats.stopTimer()

		base.Assert(func() bool { return x.node.Alias().Equals(x.node.Buildable.Alias()) })

		state.stats.add(&x.stats)
		x.buildGraphWritePort.stats.atomic_add(&x.stats)
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
		x.node.Stamp = MakeTimedBuildFingerprint(x.annotations.Timestamp, x.node.Buildable)
		base.Assert(func() bool { return x.node.Stamp.Content.Valid() })

		// need to save the build graph if build stamp changed
		if x.previousStamp != x.node.Stamp {
			x.makeDirty("build stamp updated")
		}

		return BuildResult{
			BuildAlias: x.node.BuildAlias,
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

		// abort every other build if stop-on-error is enabled
		if GetCommandFlags().StopOnError.Get() {
			x.Abort(err)
		}

		return BuildResult{
			BuildAlias: x.node.BuildAlias,
			Buildable:  x.node.Buildable,
			BuildStamp: BuildStamp{},
		}, true, err
	}
}

func (x *buildExecuteContext) Alias() BuildAlias {
	return x.node.Alias()
}
func (x *buildExecuteContext) GetBuildOptions() *BuildOptions {
	return x.options
}
func (x *buildExecuteContext) GetStaticDependencyBuildResults() []BuildResult {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	return x.staticResults
}
func (x *buildExecuteContext) Annotate(annotations ...BuildAnnotateFunc) {
	x.barrier.Lock()
	defer x.barrier.Unlock()

	for _, it := range annotations {
		it(&x.annotations)
	}
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

func (x *buildExecuteContext) NeedBuildResult(results ...BuildResult) {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	for _, br := range results {
		x.node.addDynamic_AssumeLocked(br.BuildAlias, br.BuildStamp)
	}
}

func (x *buildExecuteContext) NeedBuildAliasables(n int, buildAliasables func(int) BuildAliasable, onBuildResult func(int, BuildResult) error) error {
	return x.buildMany(n,
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(buildAliasables(i).Alias())
		},
		func(i int, br BuildResult) error {
			x.NeedBuildResult(br)
			return onBuildResult(i, br)
		})
}

func (x *buildExecuteContext) dependsOn_AssumeLocked(n int, aliases func(int) BuildAlias, opts ...BuildOptionFunc) error {
	base.Assert(func() bool { return n > 0 })

	return x.buildMany(n,
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(aliases(i))
		},
		func(i int, br BuildResult) error {
			x.node.addDynamic_AssumeLocked(br.BuildAlias, br.BuildStamp)
			return nil
		},
		opts...)
}
func (x *buildExecuteContext) DependsOn(aliases ...BuildAlias) error {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	return x.dependsOn_AssumeLocked(len(aliases), func(i int) BuildAlias { return aliases[i] },
		OptionBuildRecurse(x.options, x.node))
}
func (x *buildExecuteContext) NeedBuildable(aliasable BuildAliasable, opts ...BuildOptionFunc) (Buildable, error) {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	_, future := x.Build(aliasable, OptionBuildRecurse(x.options, x.node))

	if result, err := future.Join().Get(); err == nil {
		x.node.addDynamic_AssumeLocked(result.BuildAlias, result.BuildStamp)
		return result.Buildable, nil
	} else {
		return nil, err
	}
}
func (x *buildExecuteContext) NeedFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	future := PrepareBuildFactory(x, factory,
		OptionBuildRecurse(x.options, x.node),
		OptionBuildOverride(opts...))

	if result, err := future.Join().Get(); err == nil {
		x.node.addDynamic_AssumeLocked(result.BuildAlias, result.BuildStamp)
		return result.Buildable, nil
	} else {
		return nil, err
	}
}
func (x *buildExecuteContext) needFactoriesFunc(n int, factories func(int) BuildFactory) error {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	return x.buildMany(n,
		func(i int, bo *BuildOptions) (*buildNode, error) {
			return buildInit(x.buildGraphWritePort, factories(i), OptionBuildCopy(bo))
		},
		func(i int, br BuildResult) error {
			x.node.addDynamic_AssumeLocked(br.BuildAlias, br.BuildStamp)
			return nil
		},
		OptionBuildRecurse(x.options, x.node))
}
func (x *buildExecuteContext) NeedFactories(factories ...BuildFactory) error {
	return x.needFactoriesFunc(len(factories), func(i int) BuildFactory {
		return factories[i]
	})
}
func (x *buildExecuteContext) NeedFiles(files ...Filename) error {
	return x.needFactoriesFunc(len(files), func(i int) BuildFactory {
		return BuildFile(files[i])
	})
}
func (x *buildExecuteContext) NeedDirectories(directories ...Directory) error {
	return x.needFactoriesFunc(len(directories), func(i int) BuildFactory {
		return BuildDirectory(directories[i])
	})
}

func (x *buildExecuteContext) OutputFile(files ...Filename) error {
	base.Assert(func() bool { return len(files) > 0 })

	// files are treated as an exception: we build them outside of build scope, without using a future
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	for _, it := range files {
		it = it.Normalize()

		base.LogDebug(LogBuildGraph, "%v: output file %q", x.Alias(), it)

		// create output file with a static dependency pointing to its creator (e.g x.node here)
		file, err := PrepareOutputFile(x.buildGraphWritePort, it, MakeBuildAliases(x.node.BuildAlias),
			OptionBuildRecurse(x.options, x.node),
			// this code path always force recreates file nodes, which should be rebuild
			OptionBuildDirty,
			OptionBuildForce)
		if err != nil {
			return err
		}

		// output files are an exception: we need file build stamp to track external modifications
		// in the creator, but we also need to add a dependency from the creator on the file, creating a recursion.
		// to avoid looping (actually more dead-locking) we compute the build stamp of the file without
		// actually building its node:
		fileStamp, err := buildFileStampWithoutDeps(file.Filename)
		if err != nil {
			return err
		}
		x.node.addOutputFile_AssumeLocked(file.Alias(), fileStamp)
	}

	return nil
}
func (x *buildExecuteContext) outputFactory_AssumeLocked(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	creatorAlias := x.node.Alias()
	childFactory := WrapBuildFactory(func(bi BuildInitializer) (Buildable, error) {
		// add caller node as a static dependency
		if err := bi.DependsOn(creatorAlias); err != nil {
			return nil, err
		}
		return factory.Create(bi)
	})

	outputNode, err := buildInit(x.buildGraphWritePort, childFactory, opts...)
	if err != nil {
		return nil, err
	}

	base.LogDebug(LogBuildGraph, "%v: outputs node %q", x.Alias(), outputNode.Alias())
	x.node.addOutputNode_AssumeLocked(outputNode.Alias())

	return outputNode.Buildable, nil
}

func (x *buildExecuteContext) OutputNode(factories ...BuildFactory) error {
	if len(factories) == 0 {
		return nil
	}

	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	for _, it := range factories {
		if _, err := x.outputFactory_AssumeLocked(it,
			OptionBuildRecurse(x.options, x.node),
			OptionBuildForce); err != nil {
			return err
		}
	}

	return nil
}

func (x *buildExecuteContext) OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	x.lock_for_dependency()
	defer x.unlock_for_dependency()

	return x.outputFactory_AssumeLocked(factory,
		OptionBuildRecurse(x.options, x.node),
		OptionBuildOverride(opts...),
		OptionBuildForce)
}

/***************************************
 * Build Graph Context
 ***************************************/

type buildGraphContext struct {
	*buildGraphWritePort
	options   *BuildOptions
	timestamp time.Time
}

func makeBuildGraphContext(g *buildGraphWritePort, options *BuildOptions) buildGraphContext {
	return buildGraphContext{buildGraphWritePort: g, options: options, timestamp: time.Now()}
}

func (x buildGraphContext) GetBuildOptions() *BuildOptions                         { return x.options }
func (x buildGraphContext) GetStaticDependencyBuildResults() (empty []BuildResult) { return }

func (x buildGraphContext) NeedBuildResult(...BuildResult) { /*NOOP*/ }
func (x buildGraphContext) DependsOn(aliases ...BuildAlias) error {
	return x.buildMany(len(aliases),
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(aliases[i])
		},
		func(i int, br BuildResult) error {
			return nil
		},
		OptionBuildRecurse(x.options, nil))
}
func (x *buildGraphContext) NeedBuildAliasables(n int, buildAliasables func(int) BuildAliasable, onBuildResult func(int, BuildResult) error) error {
	return x.buildMany(n,
		func(i int, _ *BuildOptions) (*buildNode, error) {
			return x.findNode(buildAliasables(i).Alias())
		},
		func(i int, br BuildResult) error {
			return onBuildResult(i, br)
		})
}
func (x buildGraphContext) NeedBuildable(aliasable BuildAliasable, opts ...BuildOptionFunc) (Buildable, error) {
	_, future := x.Build(aliasable, opts...)
	if result, err := future.Join().Get(); err == nil {
		return result.Buildable, nil
	} else {
		return nil, err
	}
}

func (x buildGraphContext) NeedFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	result, err := PrepareBuildFactory(x.buildGraphWritePort, factory,
		OptionBuildRecurse(x.options, nil),
		OptionBuildOverride(opts...)).Join().Get()
	if err == nil {
		return result.Buildable, nil
	} else {
		return nil, err
	}
}

func (x buildGraphContext) needFactoriesFunc(n int, factories func(int) BuildFactory, opts ...BuildOptionFunc) error {
	return base.ParallelJoin(
		func(i int, result BuildResult) error {
			return nil
		},
		base.Range(func(i int) base.Future[BuildResult] {
			return PrepareBuildFactory(x.buildGraphWritePort, factories(i),
				OptionBuildRecurse(x.options, nil),
				OptionBuildOverride(opts...))
		}, n)...)
}

func (x buildGraphContext) NeedFactories(factories ...BuildFactory) error {
	return x.needFactoriesFunc(len(factories), func(i int) BuildFactory {
		return factories[i]
	})
}
func (x buildGraphContext) NeedFiles(filenames ...Filename) error {
	return x.needFactoriesFunc(len(filenames), func(i int) BuildFactory {
		return BuildFile(filenames[i])
	})
}
func (x buildGraphContext) NeedDirectories(directories ...Directory) error {
	return x.needFactoriesFunc(len(directories), func(i int) BuildFactory {
		return BuildDirectory(directories[i])
	})
}

func (x buildGraphContext) OutputFile(filenames ...Filename) error {
	return x.needFactoriesFunc(len(filenames), func(i int) BuildFactory {
		return BuildFile(filenames[i])
	}, OptionBuildDirty, OptionBuildForce)
}
func (x buildGraphContext) OutputNode(factories ...BuildFactory) error {
	return x.needFactoriesFunc(len(factories), func(i int) BuildFactory {
		return factories[i]
	}, OptionBuildForce)
}
func (x buildGraphContext) OutputFactory(factory BuildFactory, opts ...BuildOptionFunc) (Buildable, error) {
	return x.NeedFactory(factory, OptionBuildOverride(opts...), OptionBuildForce)
}

func (x *buildGraphContext) Annotate(annotations ...BuildAnnotateFunc) {
	if base.DEBUG_ENABLED {
		var ba BuildAnnotations
		for _, it := range annotations {
			it(&ba)
		}
		for _, s := range ba.Comments {
			base.LogDebug(LogBuildGraph, "build comment annotation: %v", s)
		}
		if ba.Dirty {
			base.LogDebug(LogBuildGraph, "build dirty annotation")
		}
		if ba.Mute {
			base.LogDebug(LogBuildGraph, "build mute annotation")
		}
		if ba.Timestamp != (time.Time{}) {
			base.LogDebug(LogBuildGraph, "build timestamp: %v", ba.Timestamp)
		}
	}
}

func (x *buildGraphContext) OnBuilt(e func(BuildNode) error) {
	base.UnreachableCode()
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
func OptionBuildCopy(value *BuildOptions) BuildOptionFunc {
	return func(opts *BuildOptions) {
		*opts = *value
	}
}
func OptionBuildOverride(overrides ...BuildOptionFunc) BuildOptionFunc {
	return func(opts *BuildOptions) {
		opts.Init(overrides...)
	}
}
func OptionBuildRecurse(value *BuildOptions, node BuildNode) BuildOptionFunc {
	return func(opts *BuildOptions) {
		*opts = value.Recurse(node)
	}
}

/***************************************
 * Build Annotations
 ***************************************/

func AnnocateBuildComment(comments ...string) BuildAnnotateFunc {
	return func(ba *BuildAnnotations) {
		ba.Comments = append(ba.Comments, comments...)
	}
}
func AnnocateBuildCommentf(format string, a ...any) BuildAnnotateFunc {
	return func(ba *BuildAnnotations) {
		ba.Comments = append(ba.Comments, fmt.Sprintf(format, a...))
	}
}
func AnnocateBuildCommentWith[T fmt.Stringer](stringers ...T) BuildAnnotateFunc {
	return func(ba *BuildAnnotations) {
		for _, it := range stringers {
			ba.Comments = append(ba.Comments, it.String())
		}
	}
}
func AnnocateBuildDirty(ba *BuildAnnotations) {
	ba.Dirty = true
}
func AnnocateBuildDirtyIf(dirty bool) BuildAnnotateFunc {
	return func(ba *BuildAnnotations) {
		ba.Dirty = dirty
	}
}
func AnnocateBuildMute(ba *BuildAnnotations) {
	ba.Mute = true
}
func AnnocateBuildMuteIf(mute bool) BuildAnnotateFunc {
	return func(ba *BuildAnnotations) {
		ba.Mute = mute
	}
}
func AnnocateBuildTimestamp(timestamp time.Time) BuildAnnotateFunc {
	return func(ba *BuildAnnotations) {
		ba.Timestamp = timestamp
	}
}

/***************************************
 * Launch build for a node
 ***************************************/

func (g *buildGraphWritePort) launchBuild(node *buildNode, options *BuildOptions) base.Future[BuildResult] {
	base.AssertErr(func() error {
		if alias := node.Buildable.Alias(); alias.Equals(node.BuildAlias) {
			return nil
		} else {
			return fmt.Errorf("%v: node alias do not match buildable %q\n\t-> %v", node.BuildAlias, alias, node.Buildable)
		}
	})
	base.AssertErr(func() error {
		relateOutp := strings.Builder{}
		if options.RelatesVerbose(node, 0, &relateOutp) {
			return fmt.Errorf("build cyclic dependency in %q\n%s", node, relateOutp.String())
		}
		return nil
	})

	if BUILDGRAPH_ENABLE_CHECKS {
		relateOutp := strings.Builder{}
		if options.RelatesVerbose(node, 0, &relateOutp) {
			base.LogPanic(LogBuildGraph, "build cyclic dependency in %q\n%s", node, relateOutp.String())
		}
		base.LogTrace(LogBuildGraph, "buildgraph: launch build of <%T> %q\n%s", node.Buildable, node.Alias(), relateOutp.String())
	}

	var newSate buildState
	newSate.buildNode = node
	state, _ := g.state.FindOrAdd(node.Alias(), &newSate)

	if future := state.future.Load(); future != nil {
		if options.Force {
			future.Join()
		} else {
			return future
		}
	}

	node.Lock()
	defer node.Unlock()

	if future := state.future.Load(); future != nil { // check if another thread already launched the node
		if options.Force {
			future.Join()
		} else {
			return future
		}
	}

	newFuture := base.MakeFuture(func() (BuildResult, error) {
		g.onBuildNodeStart_ThreadSafe(state)
		defer g.onBuildNodeFinished_ThreadSafe(state)

		context := makeBuildExecuteContext(g, node, options)
		result, built, err := context.Execute(state)

		if err == nil && built {
			err = options.OnBuilt.Invoke(node)
		}

		if err != nil {
			switch err.(type) {
			case buildAbortError, buildDependencyError:
			default: // failed dependency errors are only printed once
				base.LogError(LogBuildGraph, "%v", err)
			}
			return result, err
		}

		if built {
			changed := (result.BuildStamp != context.previousStamp)

			if base.IsLogLevelActive(base.LOG_VERYVERBOSE) || !(node.IsMuted() || context.annotations.Mute) {
				base.LogInfo(
					LogBuildGraph,
					"%s%s %q in %v%s",
					base.Blend(``, `force `, options.Force),
					base.Blend(`build`, `update`, changed),
					node.BuildAlias,
					context.stats.Duration.Exclusive,
					base.MakeStringer(func() (annotations string) {
						if len(context.annotations.Comments) > 0 {
							annotations = fmt.Sprint(` (`, strings.Join(context.annotations.Comments, `, `), `)`)
						}
						return
					}))
			}

			if changed {
				base.LogDebug(LogBuildGraph, "%v: new build stamp for [%T]\n\tnew: %v\n\told: %v", node.BuildAlias, result.Buildable, result.BuildStamp, context.previousStamp)
			}

		} else if options.Force {
			base.LogVerbose(LogBuildGraph, "force up-to-date %q", node.BuildAlias)
		} else {
			base.LogVerbose(LogBuildGraph, "up-to-date %q", node.BuildAlias)
		}

		return result, err
	})

	state.future.Store(newFuture)
	base.LogPanicIfFailed(LogBuildGraph, options.OnLaunched.Invoke(node))

	return newFuture
}

func (g *buildGraphWritePort) buildMany(n int, nodes func(int, *BuildOptions) (*buildNode, error), onResults func(int, BuildResult) error, opts ...BuildOptionFunc) error {
	switch n {
	case 0:
		return nil

	case 1:
		bo := NewBuildOptions(opts...)

		node, err := nodes(0, &bo)
		if err != nil {
			return err
		}

		future := g.launchBuild(node, &bo)

		result, err := future.Join().Get()
		if err == nil {
			err = onResults(0, result)
		}

		return err

	default:
		futures := make([]base.Future[BuildResult], n)
		for i := range futures {
			bo := NewBuildOptions(opts...)
			if node, err := nodes(i, &bo); err == nil {
				futures[i] = g.launchBuild(node, &bo)
			} else {
				return err
			}
		}

		return base.ParallelJoin(onResults, futures...)
	}
}
func (g *buildGraphWritePort) launchBuildMany(n int, nodes func(int, *BuildOptions) (*buildNode, error), opts ...BuildOptionFunc) base.Future[[]BuildResult] {
	switch n {
	case 0:
		return base.MakeFutureLiteral([]BuildResult{})

	case 1:
		bo := NewBuildOptions(opts...)
		node, err := nodes(0, &bo)
		if err != nil {
			return base.MakeFutureError[[]BuildResult](err)
		}

		future := g.launchBuild(node, &bo)

		return base.MapFuture(future, func(br BuildResult) ([]BuildResult, error) {
			return []BuildResult{br}, nil
		})

	default:
		return base.MakeFuture(func() (results []BuildResult, err error) {
			results = make([]BuildResult, n)
			err = g.buildMany(n, nodes, func(i int, br BuildResult) error {
				results[i] = br
				return nil
			}, opts...)
			return
		})
	}
}
