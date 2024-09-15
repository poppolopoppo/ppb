package cmd

import (
	"fmt"
	"io"
	"reflect"

	"github.com/poppolopoppo/ppb/internal/base"
	internal_io "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandGraphviz = newExportNodesCommand(
	"Debug",
	"graphviz",
	"dump build node to graphviz .dot file format",
	func(cc CommandContext, args *ExportNodeArgs[BuildAlias, *BuildAlias]) error {
		bg := CommandEnv.BuildGraph()

		nodes := make([]BuildNode, len(args.Aliases))
		for i, a := range args.Aliases {
			var err error
			if nodes[i], err = bg.Expect(a); err != nil {
				return err
			}
		}

		return args.WithOutput(func(w io.Writer) error {
			gvz := newBuildGraphViz(bg, w, args.Minify.Get())
			gvz.Digraph("G", func() {
				for _, node := range nodes {
					gvz.Visit(node,
						internal_io.OptionGraphVizFontSize(36),
						internal_io.OptionGraphVizFillColor("red"),
						internal_io.OptionGraphVizFontColor("yellow"),
						internal_io.OptionGraphVizScale(2))
				}
				gvz.CloseSubGraphs()
			},
				internal_io.OptionGraphVizCustom(`rankdir="LR"`),
				internal_io.OptionGraphVizScale(5),
				internal_io.OptionGraphVizFontName("Helvetica,Arial,sans-serif"),
				internal_io.OptionGraphVizFontSize(9))

			return nil
		})
	})

type buildGraphVizEdge struct {
	from, to string
	internal_io.GraphVizOptions
}
type buildGraphVizNode struct {
	id string
	internal_io.GraphVizOptions
}

type buildGraphViz struct {
	internal_io.GraphVizFile
	graph     BuildGraph
	visited   map[BuildNode]string
	subgraphs map[string][]buildGraphVizNode
	edges     []buildGraphVizEdge
	clustered bool
	minify    bool
}

func newBuildGraphViz(graph BuildGraph, w io.Writer, minify bool) buildGraphViz {
	return buildGraphViz{
		graph:        graph,
		GraphVizFile: internal_io.NewGraphVizFile(w),
		visited:      make(map[BuildNode]string),
		subgraphs:    make(map[string][]buildGraphVizNode),
		edges:        make([]buildGraphVizEdge, 0),
		minify:       minify,
	}
}
func (x *buildGraphViz) CloseSubGraphs() error {
	for id, nodes := range x.subgraphs {
		x.SubGraph(id, func() {
			for _, node := range nodes {
				x.Node(node.id, internal_io.OptionGraphVizOptions(&node.GraphVizOptions))
			}
		})
	}
	for _, edge := range x.edges {
		x.Edge(edge.from, edge.to, internal_io.OptionGraphVizOptions(&edge.GraphVizOptions))
	}
	return nil
}
func (x *buildGraphViz) CompoundNode(subgraph, id string, options *internal_io.GraphVizOptions) {
	if x.clustered {
		nodes := x.subgraphs[subgraph]
		nodes = append(nodes, buildGraphVizNode{id: id, GraphVizOptions: *options})
		x.subgraphs[subgraph] = nodes
	} else {
		x.Node(id, internal_io.OptionGraphVizOptions(options))
	}
}
func (x *buildGraphViz) CompoundEdge(from, to string, options ...internal_io.GraphVizOptionFunc) {
	if from == "" || to == "" {
		return // don't add edge from/to a minified node
	}
	if x.clustered {
		x.edges = append(x.edges, buildGraphVizEdge{from: from, to: to, GraphVizOptions: internal_io.NewGraphVizOptions(options...)})
	} else {
		x.Edge(from, to, options...)
	}
}
func (x *buildGraphViz) Visit(node BuildNode, userOptions ...internal_io.GraphVizOptionFunc) string {
	if id, ok := x.visited[node]; ok {
		return id
	}

	id := node.Alias().String()
	x.visited[node] = id

	options := internal_io.GraphVizOptions{}
	options.Label = trimNodeLabel(id)
	options.Tooltip = id

	minified := true

	switch buildable := node.GetBuildable().(type) {
	case *FileDependency:
		options.Color = "#AAE4B580"
		options.Shape = internal_io.GRAPHVIZ_Note
		options.FontSize = 7
	case *DirectoryDependency:
		options.Color = "#AAFACD80"
		options.Shape = internal_io.GRAPHVIZ_Folder
		options.FontSize = 7
	case BuildableSourceFile:
		options.Color = "#7BE86E50"
		options.Shape = internal_io.GRAPHVIZ_Component
		options.FontSize = 7
	case BuildableSourceDirectory:
		options.Color = "#7B68EE50"
		options.Shape = internal_io.GRAPHVIZ_Component
		options.FontSize = 7

	default:
		ty := reflect.TypeOf(buildable)
		color := base.NewColorFromStringHash(ty.String()).Quantize(true)

		options.Color = color.ToHTML(0x80)
		options.Style = internal_io.GRAPHVIZ_Filled
		options.Shape = internal_io.GRAPHVIZ_Cds

		minified = false
	}

	// ignore file/directory nodes when minify is enabled
	if minified && x.minify {
		id = ""
		x.visited[node] = id
		return id
	}

	options.Init(userOptions...)
	category := "cluster" + base.SanitizeIdentifier(reflect.TypeOf(node.GetBuildable()).String())
	x.CompoundNode(category, id, &options)

	for _, dep := range x.graph.GetStaticDependencies(node) {
		x.CompoundEdge(id, x.Visit(dep), internal_io.OptionGraphVizColor("#1E90FFFF"), internal_io.OptionGraphVizWeight(2))
	}
	for _, dep := range x.graph.GetDynamicDependencies(node) {
		x.CompoundEdge(id, x.Visit(dep), internal_io.OptionGraphVizColor("#E16F00FF"), internal_io.OptionGraphVizWeight(1))
	}
	// for _, dep := range x.graph.GetOutputDependencies(node) {
	// 	x.Edge(id, x.Visit(dep), OptionGraphVizColor("#F4A46090"), OptionGraphVizWeight(3))
	// }

	return id
}

func trimNodeLabel(id string) string {
	if len(id) > 40 {
		return fmt.Sprint(id[:18], `[..]`, id[len(id)-18:])
	} else {
		return id
	}
}
