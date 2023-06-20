package utils

import (
	"fmt"
	"sync"

	"github.com/poppolopoppo/ppb/internal/base"
)

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

type BuildFactory interface {
	Create(BuildInitializer) (Buildable, error)
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
	return FindBuildable[T](CommandEnv.BuildGraph(), alias)
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
	Prepare(BuildGraph, ...BuildOptionFunc) base.Future[T]
	Build(BuildGraph, ...BuildOptionFunc) base.Result[T]
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
	base.LogPanicIfFailed(LogBuildGraph, err)
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
func (x buildFactoryWrapped[T]) Prepare(bg BuildGraph, options ...BuildOptionFunc) base.Future[T] {
	bo := NewBuildOptions(options...)
	future := PrepareBuildFactory(bg, x, &bo)
	return base.MapFuture(future, func(it BuildResult) (T, error) {
		return it.Buildable.(T), nil
	})
}
func (x buildFactoryWrapped[T]) Build(bg BuildGraph, options ...BuildOptionFunc) base.Result[T] {
	return x.Prepare(bg, options...).Join()
}

func InitBuildFactory(bg BuildGraph, factory BuildFactory, options *BuildOptions) (*buildNode, error) {
	return buildInit(bg.(*buildGraph), factory, options)
}
func PrepareBuildFactory(bg BuildGraph, factory BuildFactory, options *BuildOptions) base.Future[BuildResult] {
	node, err := InitBuildFactory(bg, factory, options)
	if err != nil {
		return base.MakeFutureError[BuildResult](err)
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
	base.Assert(func() bool { return !base.IsNil(buildable) })

	node := g.Create(buildable, context.staticDeps, OptionBuildStruct(options))

	base.Assert(func() bool { return node.Alias().Equals(buildable.Alias()) })
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
