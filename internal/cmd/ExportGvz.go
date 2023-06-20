package cmd

import (
	"fmt"
	"io"
	"reflect"

	//lint:ignore ST1001 ignore dot imports warning
	"github.com/poppolopoppo/ppb/internal/base"
	. "github.com/poppolopoppo/ppb/internal/io"

	//lint:ignore ST1001 ignore dot imports warning
	. "github.com/poppolopoppo/ppb/utils"
)

var CommandGraphviz = newCompletionCommand[BuildAlias, *BuildAlias](
	"Export",
	"export-gvz",
	"dump build node to graphviz .dot",
	func(cc CommandContext, ca *CompletionArgs[BuildAlias, *BuildAlias]) error {
		bg := CommandEnv.BuildGraph()

		nodes := make([]BuildNode, len(ca.Inputs))
		for i, a := range ca.Inputs {
			node := bg.Find(a)
			if node != nil {
				nodes[i] = node
			} else {
				return BuildableNotFound{Alias: a}
			}
		}

		return openCompletion(ca, func(w io.Writer) error {
			gvz := newBuildGraphViz(bg, w)
			gvz.Digraph("G", func() {
				for _, node := range nodes {
					gvz.Visit(node, OptionGraphVizFontSize(36), OptionGraphVizFillColor("red"), OptionGraphVizFontColor("yellow"), OptionGraphVizScale(2))
				}
				gvz.CloseSubGraphs()
			},
				OptionGraphVizCustom(`rankdir="LR"`),
				OptionGraphVizScale(5),
				OptionGraphVizFontName("Helvetica,Arial,sans-serif"),
				OptionGraphVizFontSize(9))

			return nil
		})
	})

type buildGraphVizEdge struct {
	from, to string
	GraphVizOptions
}
type buildGraphVizNode struct {
	id string
	GraphVizOptions
}

type buildGraphViz struct {
	GraphVizFile
	graph     BuildGraph
	visited   map[BuildNode]string
	clustered bool
	subgraphs map[string][]buildGraphVizNode
	edges     []buildGraphVizEdge
}

func newBuildGraphViz(graph BuildGraph, w io.Writer) buildGraphViz {
	return buildGraphViz{
		graph:        graph,
		GraphVizFile: NewGraphVizFile(w),
		visited:      make(map[BuildNode]string),
		subgraphs:    make(map[string][]buildGraphVizNode),
		edges:        make([]buildGraphVizEdge, 0),
	}
}
func (x *buildGraphViz) CloseSubGraphs() error {
	for id, nodes := range x.subgraphs {
		x.SubGraph(id, func() {
			for _, node := range nodes {
				x.Node(node.id, OptionGraphVizOptions(&node.GraphVizOptions))
			}
		})
	}
	for _, edge := range x.edges {
		x.Edge(edge.from, edge.to, OptionGraphVizOptions(&edge.GraphVizOptions))
	}
	return nil
}
func (x *buildGraphViz) CompoundNode(subgraph, id string, options *GraphVizOptions) {
	if x.clustered {
		nodes, _ := x.subgraphs[subgraph]
		nodes = append(nodes, buildGraphVizNode{id: id, GraphVizOptions: *options})
		x.subgraphs[subgraph] = nodes
	} else {
		x.Node(id, OptionGraphVizOptions(options))
	}
}
func (x *buildGraphViz) CompoundEdge(from, to string, options ...GraphVizOptionFunc) {
	if x.clustered {
		x.edges = append(x.edges, buildGraphVizEdge{from: from, to: to, GraphVizOptions: NewGraphVizOptions(options...)})
	} else {
		x.Edge(from, to, options...)
	}
}
func (x *buildGraphViz) Visit(node BuildNode, userOptions ...GraphVizOptionFunc) string {
	if id, ok := x.visited[node]; ok {
		return id
	}

	id := node.Alias().String()
	x.visited[node] = id

	options := GraphVizOptions{}
	options.Label = trimNodeLabel(id)
	options.Tooltip = id

	switch buildable := node.GetBuildable().(type) {
	case *Filename:
		options.Color = "#AAE4B580"
		options.Shape = GRAPHVIZ_Note
		options.FontSize = 7
	case *Directory:
		options.Color = "#AAFACD80"
		options.Shape = GRAPHVIZ_Folder
		options.FontSize = 7
	case *DirectoryCreator, *DirectoryGlob, *DirectoryList:
		options.Color = "#7B68EE50"
		options.Shape = GRAPHVIZ_Component
		options.FontSize = 7
	default:
		ty := reflect.TypeOf(buildable)
		digest := base.StringFingerprint(ty.String())

		color := base.NewColor3f(
			float64(digest[len(digest)-1])/0xFF,
			0.7,
			0.8,
		).HslToRgb().Quantize(true)

		options.Color = color.ToHTML(0x80)
		options.Style = GRAPHVIZ_Filled
		options.Shape = GRAPHVIZ_Cds
	}

	options.Init(userOptions...)
	category := "cluster" + base.SanitizeIdentifier(reflect.TypeOf(node.GetBuildable()).String())
	x.CompoundNode(category, id, &options)

	for _, dep := range x.graph.GetStaticDependencies(node) {
		x.CompoundEdge(id, x.Visit(dep), OptionGraphVizColor("#1E90FF30"), OptionGraphVizWeight(2))
	}
	for _, dep := range x.graph.GetDynamicDependencies(node) {
		x.CompoundEdge(id, x.Visit(dep), OptionGraphVizColor("#E16F0030"), OptionGraphVizWeight(1))
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
