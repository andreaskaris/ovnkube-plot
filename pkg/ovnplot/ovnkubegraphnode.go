package ovnplot

import (
	"strconv"

	"github.com/emicklei/dot"
)

// OvnKubeGraphNode is a custom type based on dot.Node. It allows us to define a standardized format for the different
// types, such as Switch(), Router(), Leaf(), Invisible().
type OvnKubeGraphNode dot.Node

func (d OvnKubeGraphNode) Switch() dot.Node {
	d.Attr("shape", "diamond")
	d.Attr("color", "seagreen")
	return dot.Node(d)
}

func (d OvnKubeGraphNode) Router() dot.Node {
	d.Attr("shape", "octagon")
	d.Attr("color", "salmon")
	return dot.Node(d)
}

func (d OvnKubeGraphNode) Leaf() dot.Node {
	d.Attr("shape", "oval")
	return dot.Node(d)
}

func (d OvnKubeGraphNode) Invisible() dot.Node {
	d.Attr("shape", "point")
	return dot.Node(d)
}

// NodeList holds all nodes for a given graph.
type NodeList struct {
	nodes map[string]dot.Node
	g     *dot.Graph
}

// NewNodeList initializes a new node list.
func NewNodeList(g *dot.Graph) *NodeList {
	return &NodeList{
		make(map[string]dot.Node),
		g,
	}
}

// GetNode retrieves a node or if it does not exist, creates it.
func (nl *NodeList) GetNode(name string) dot.Node {
	if node, ok := nl.nodes[name]; ok {
		return node
	}
	nl.nodes[name] = nl.g.Node(name)
	return nl.nodes[name]
}

func (nl *NodeList) GetSwitch(name string) dot.Node {
	return OvnKubeGraphNode(nl.GetNode(name)).Switch()
}

func (nl *NodeList) GetRouter(name string) dot.Node {
	return OvnKubeGraphNode(nl.GetNode(name)).Router()
}

func (nl *NodeList) GetLeaf(name string) dot.Node {
	return OvnKubeGraphNode(nl.GetNode(name)).Leaf()
}

func (nl *NodeList) GetInvisible(name string) dot.Node {
	return OvnKubeGraphNode(nl.GetNode(name)).Invisible()
}

// DrawLevels draws the levels for the compact graph.
func (nl *NodeList) DrawLevels(level int) {
	if level == 0 {
		return
	}
	if level == 1 {
		nl.GetNode("1")
		return
	}
	e := nl.GetNode("1").Edge(nl.GetNode("2"))
	for i := 3; i <= level; i++ {
		e = e.Edge(nl.GetNode(strconv.Itoa(i)))
	}
}
