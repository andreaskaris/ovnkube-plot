package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"text/tabwriter"
	"text/template"

	"k8s.io/klog/v2"
	kexec "k8s.io/utils/exec"

	goovn "github.com/ebay/go-ovn"

	"github.com/urfave/cli/v2"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/config"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"

	dot "github.com/emicklei/dot"
)

const (
	// CustomAppHelpTemplate helps in grouping options to ovnkube
	CustomDBCheckAppHelpTemplate = `NAME:
   {{.Name}} - {{.Usage}}

USAGE:
   {{.HelpName}} [global options]

VERSION:
   {{.Version}}{{if .Description}}

DESCRIPTION:
   {{.Description}}{{end}}

COMMANDS:{{range .VisibleCategories}}{{if .Name}}

   {{.Name}}:{{end}}{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}

GLOBAL OPTIONS:{{range $title, $category := getFlagsByCategory}}
   {{upper $title}}
   {{range $index, $option := $category}}{{if $index}}
   {{end}}{{$option}}{{end}}
   {{end}}`
)

var format string
var filter string

// OVNK8sFeatureFlags capture OVN-Kubernetes feature related options
var customFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "format",
		Usage:       "The output format ('compact' or 'detailed')",
		Destination: &format,
	},
	&cli.StringFlag{
		Name:        "filter",
		Usage:       "Show only matching nodes",
		Destination: &filter,
	},
}

func getFlagsByCategory() map[string][]cli.Flag {
	m := map[string][]cli.Flag{}
	m["OVN Northbound DB Options"] = config.OvnNBFlags
	m["OVN Kubeplot Options"] = customFlags
	return m
}

// borrowed from cli packages' printHelpCustom()
func printOvnKubePlotHelp(out io.Writer, templ string, data interface{}, customFunc map[string]interface{}) {
	funcMap := template.FuncMap{
		"join":               strings.Join,
		"upper":              strings.ToUpper,
		"getFlagsByCategory": getFlagsByCategory,
	}
	for key, value := range customFunc {
		funcMap[key] = value
	}

	w := tabwriter.NewWriter(out, 1, 8, 2, ' ', 0)
	t := template.Must(template.New("help").Funcs(funcMap).Parse(templ))
	err := t.Execute(w, data)
	if err == nil {
		_ = w.Flush()
	}
}

func main() {
	cli.HelpPrinterCustom = printOvnKubePlotHelp
	c := cli.NewApp()
	c.Name = "ovnkube-plot"
	c.Usage = "plot ovnkube network in a human readable way"
	c.Version = "0.0.1"
	c.CustomAppHelpTemplate = CustomDBCheckAppHelpTemplate
	c.Flags = config.GetFlags(customFlags)

	c.Action = func(c *cli.Context) error {
		return runOvnKubePlot(c)
	}

	if err := c.Run(os.Args); err != nil {
		klog.Exit(err)
	}
}

func runOvnKubePlot(ctx *cli.Context) error {
	exec := kexec.New()
	_, err := config.InitConfig(ctx, exec, nil)
	if err != nil {
		return err
	}

	var ovnNBClient goovn.Client
	if ovnNBClient, err = util.NewOVNNBClient(); err != nil {
		return err
	}

	if filter == "" {
		filter = ".*"
	}
	var output string
	if format == "detailed" {
		output, err = detailedPlot(&ovnNBClient, filter)
	} else {
		output, err = compactPlot(&ovnNBClient, filter)
	}

	if err != nil {
		return err
	}

	fmt.Println(output)

	return nil
}

// Draw every switch and router as a subgraph. Add interfaces and routes as individual nodes under the subgraphs.
// Then, connect all interfaces to each other, with a single connection between pairs
// Ideal when using the filter expression and when filtering for a very restricted number of nodes
func detailedPlot(client *goovn.Client, filter string) (string, error) {
	g := dot.NewGraph(dot.Directed)

	g.Attr("rankdir", "LR")
	g.EdgeInitializer(func(e dot.Edge) {
		e.Attr("fontname", "arial")
		e.Attr("fontsize", "9")
		e.Attr("penwidth", "4.0")
		e.Attr("arrowhead", "none")
	})

	var lrs []*goovn.LogicalRouter
	lrs, err := (*client).LRList()
	if err != nil {
		return "", err
	}

	// first, draw all subgraphs for routers
	routers := map[string]*dot.Graph{}
	routerPorts := map[string]dot.Node{}
	for _, lr := range lrs {
		if matched, _ := regexp.MatchString("^join$|^ovn_cluster_router$|^node_local_switch$|"+filter, lr.Name); !matched {
			continue
		}
		routers[lr.Name] = g.Subgraph(lr.Name, dot.ClusterOption{})
		routers[lr.Name].Attr("style", "filled")
		routers[lr.Name].Attr("color", "0.7 0.7 1.0")
		lrps, err := (*client).LRPList(lr.Name)
		if err != nil {
			return "", err
		}
		staticRoutes, err := (*client).LRSRList(lr.Name)
		if err == nil {
			routes := "<table BORDER='0' CELLBORDER='0' CELLSPACING='0' CELLPADDING='0'>" +
				"<tr><td>IPPrefix</td><td>Nexthop</td><td>OutputPort</td><td>Policy</td></tr>"
			for _, r := range staticRoutes {
				routes += fmt.Sprintf("<tr><td>%s</td><td>%s</td>",
					r.IPPrefix, r.Nexthop)
				if r.OutputPort != nil {
					routes += fmt.Sprintf("<td>%s</td>",
						*r.OutputPort)
				} else {
					routes += "<td></td>"
				}
				if r.Policy != nil {
					routes += fmt.Sprintf("<td>%s</td>",
						*r.Policy)
				} else {
					routes += "<td></td>"
				}
				routes += "</tr>"
			}
			routes += "</table>"
			routers[lr.Name].Node("routes-"+lr.Name).Attr("shape", "box").Attr("style", "filled").Attr("label", dot.HTML(routes))
		}
		for _, lrp := range lrps {
			routerPorts[lrp.Name] = routers[lr.Name].Node(lrp.Name)
			label := fmt.Sprintf("<table><tr><td>Name</td><td>Networks</td><td>MAC</td></tr>"+
				"<tr><td port='main'>%s</td><td>%s</td><td>%s</td></tr></table>",
				lrp.Name,
				strings.Join(lrp.Networks, "<br />"),
				lrp.MAC)
			routerPorts[lrp.Name].Attr("label", dot.HTML(label))
			routerPorts[lrp.Name].Attr("shape", "none")
			routerPorts[lrp.Name].Attr("style", "filled")
			routerPorts[lrp.Name].Attr("color", "white")
		}
	}

	// now, draw all subgraphs for switches
	switches := map[string]*dot.Graph{}
	switchPorts := map[string]dot.Node{}
	var lss []*goovn.LogicalSwitch
	lss, err = (*client).LSList()
	if err != nil {
		return "", err
	}
	for _, ls := range lss {
		if matched, _ := regexp.MatchString("^join$|^ovn_cluster_router$|^node_local_switch$|"+filter, ls.Name); !matched {
			continue
		}
		switches[ls.Name] = g.Subgraph(ls.Name, dot.ClusterOption{})
		switches[ls.Name].Attr("style", "filled")
		switches[ls.Name].Attr("color", "0.4 1.0 0.6")
		lsps, err := (*client).LSPList(ls.Name)
		if err != nil {
			return "", err
		}
		for _, lsp := range lsps {
			switchPorts[lsp.Name] = switches[ls.Name].Node(lsp.Name)
			switchPorts[lsp.Name].Attr("shape", "box")
			switchPorts[lsp.Name].Attr("style", "filled")
			switchPorts[lsp.Name].Attr("color", "white")
			if lsp.Type == "router" {
				if matched, _ := regexp.MatchString(".*ovn_cluster_router$|.*node_local_switch$|"+filter, lsp.Name); !matched {
					continue
				}
				routerPortName := lsp.Options["router-port"].(string)
				// this is the only point where we actually draw Edges: from switchports to
				// associated router ports
				g.Edge(switchPorts[lsp.Name], routerPorts[routerPortName])
			}
		}
	}

	return g.String(), nil
}

// stuff for the compact plot starts below

// create a new type for easy formatting
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

// create a NodeList which holds all nodes for a given graph
type NodeList struct {
	nodes map[string]dot.Node
	g     *dot.Graph
}

// initialize a new node list
func NewNodeList(g *dot.Graph) *NodeList {
	return &NodeList{
		make(map[string]dot.Node),
		g,
	}
}

// Retrieve a node or if it does not exist, create it
func (nl *NodeList) GetNode(name string) dot.Node {
	if _, ok := nl.nodes[name]; ok {
		return nl.nodes[name]
	} else {
		nl.nodes[name] = nl.g.Node(name)
		return nl.nodes[name]
	}
}

// Helper function to retrieve both the LogicalRouter and the LogicalRouterPort that belong
// to string "routerPortName"
func findRouterForRouterPort(
	routerPortName string,
	client *goovn.Client) (*goovn.LogicalRouter, *goovn.LogicalRouterPort, error) {
	lrs, err := (*client).LRList()
	if err != nil {
		return nil, nil, err
	}
	for _, lr := range lrs {
		lrps, err := (*client).LRPList(lr.Name)
		if err != nil {
			return nil, nil, err
		}
		for _, lrp := range lrps {
			if lrp.Name == routerPortName {
				return lr, lrp, nil
			}
		}
	}
	return nil, nil, nil
}

// Compact Plotting
// Ideal for an overview of more complex systems as it provides a cleaner design.
// Unfortunately, this does not scale to 10s or 100s of nodes, either
func compactPlot(client *goovn.Client, filter string) (string, error) {
	g := dot.NewGraph(dot.Directed)

	g.Attr("rankdir", "LR")
	g.EdgeInitializer(func(e dot.Edge) {
		e.Attr("fontname", "arial")
		e.Attr("fontsize", "9")
		e.Attr("penwidth", "2.0")
		e.Attr("arrowhead", "none")
	})

	nl := NewNodeList(g)

	// First, draw levels - this will help keep the document formatted
	nl.GetNode("1").
		Edge(nl.GetNode("2")).
		Edge(nl.GetNode("3")).
		Edge(nl.GetNode("4")).
		Edge(nl.GetNode("5")).
		Edge(nl.GetNode("6")).
		Edge(nl.GetNode("7")).
		Edge(nl.GetNode("8")).
		Edge(nl.GetNode("9")).
		Edge(nl.GetNode("10"))

	// get the OVN LogicalSwitch table content
	lss, err := (*client).LSList()
	if err != nil {
		debug.PrintStack()
		return "", err
	}

	// "1" and "2"
	// we draw left to right and we start with all switches that
	// have chassis names; this means we exclude:
	// "join_switch", "node_local_switch", "ext_.*"
	reg1, err := regexp.Compile("^join.*|^node_local_switch$|^ext_.*")
	if err != nil {
		return "", err
	}
	reg2, err := regexp.Compile(filter)
	if err != nil {
		return "", err
	}
	for _, ls := range lss {
		if reg1.Match([]byte(ls.Name)) {
			continue
		}
		if !reg2.Match([]byte(ls.Name)) {
			continue
		}
		// get the OVN LogicalSwitchPorts for this LS
		lsps, err := (*client).LSPList(ls.Name)
		if err != nil {
			debug.PrintStack()
			return "", err
		}
		for _, lsp := range lsps {
			// "3"
			// find ovn_cluster_router programmatically, just in case
			// there should be only one router port here, ovn_cluster_router
			if lsp.Type == "router" {
				lr, lrp, err := findRouterForRouterPort(lsp.Options["router-port"].(string), client)
				label := strings.Join(lrp.Networks, ";")
				if err == nil {
					nl.GetNode(ls.Name).Edge(
						OvnKubeGraphNode(nl.GetNode(ls.Name + "spacer1")).Invisible(),
					).Label(label).Edge(
						nl.GetNode(lr.Name),
					)
					OvnKubeGraphNode(nl.GetNode(ls.Name)).Switch()
					OvnKubeGraphNode(nl.GetNode(lr.Name)).Router()
				}
			} else {
				g.Edge(nl.GetNode(lsp.Name), nl.GetNode(ls.Name))
				OvnKubeGraphNode(nl.GetNode(lsp.Name)).Leaf()
			}
		}
	}

	// 2 different designs - either one join switch, or one join switch per node
	// retrieve all join switches
	var joinSwitches []string
	reg1, err = regexp.Compile("^join.*")
	if err != nil {
		return "", err
	}
	reg2, err = regexp.Compile("^join$|" + filter)
	if err != nil {
		return "", err
	}
	for _, ls := range lss {
		if !reg1.Match([]byte(ls.Name)) {
			continue
		}
		if !reg2.Match([]byte(ls.Name)) {
			continue
		}
		joinSwitches = append(joinSwitches, ls.Name)
	}

	// "4"
	// now, add the join switch(es)
	for _, js := range joinSwitches {
		// now, add the ovn_cluster_router to the left
		// now, add all routers other than ovn_cluster_router to the right
		lsps, err := (*client).LSPList(js)
		if err != nil {
			return "", err
		}

		for _, lsp := range lsps {
			// find GR_ routers programmatically, just in case
			// there should be only one router port here, GR_ router
			if lsp.Type == "router" {
				lr, lrp, err := findRouterForRouterPort(lsp.Options["router-port"].(string), client)
				if err == nil {
					if lr.Name == "ovn_cluster_router" {
						label := strings.Join(lrp.Networks, ";")
						nl.GetNode(lr.Name).Edge(
							OvnKubeGraphNode(nl.GetNode("ovn_cluster_router" + js + "spacer1")).Invisible(),
						).Label(label).Edge(
							nl.GetNode(js),
						)
						OvnKubeGraphNode(nl.GetNode(lr.Name)).Router()
					} else if matched, _ := regexp.MatchString(filter, lr.Name); matched {
						label := strings.Join(lrp.Networks, ";")
						nl.GetNode(js).Edge(
							OvnKubeGraphNode(nl.GetNode(lr.Name + "spacer1")).Invisible(),
						).Edge(
							nl.GetNode(lr.Name),
						).Label(label)
						OvnKubeGraphNode(nl.GetNode(lr.Name)).Router()
					}
				}
			} else {
				g.Edge(nl.GetNode(js), nl.GetNode(lsp.Name))
			}
		}
		OvnKubeGraphNode(nl.GetNode(js)).Switch()
	}

	// now, add ext_* switches to the right
	reg1, err = regexp.Compile("^ext_.*")
	if err != nil {
		return "", err
	}
	reg2, err = regexp.Compile(filter)
	if err != nil {
		return "", err
	}
	for _, ls := range lss {
		if !reg1.Match([]byte(ls.Name)) {
			continue
		}
		if !reg2.Match([]byte(ls.Name)) {
			continue
		}
		// get the OVN LogicalSwitchPorts for this LS
		lsps, err := (*client).LSPList(ls.Name)
		if err != nil {
			debug.PrintStack()
			return "", err
		}
		for _, lsp := range lsps {
			// find ovn_cluster_router programmatically, just in case
			// there should be only one router port here, ovn_cluster_router
			if lsp.Type == "router" {
				lr, _, err := findRouterForRouterPort(lsp.Options["router-port"].(string), client)
				if err == nil {
					g.Edge(nl.GetNode(lr.Name), nl.GetNode(ls.Name))
					OvnKubeGraphNode(nl.GetNode(lr.Name)).Router()
				}
			} else {
				g.Edge(nl.GetNode(ls.Name), nl.GetNode(lsp.Name))
				OvnKubeGraphNode(nl.GetNode(ls.Name)).Switch()
			}
		}
	}

	// now, add the node_local_switch
	reg1, err = regexp.Compile("^node_local_switch$")
	if err != nil {
		return "", err
	}
	for _, ls := range lss {
		if !reg1.Match([]byte(ls.Name)) {
			continue
		}
		OvnKubeGraphNode(nl.GetNode(ls.Name)).Switch()
		// get the OVN LogicalSwitchPorts for this LS
		lsps, err := (*client).LSPList(ls.Name)
		if err != nil {
			debug.PrintStack()
			return "", err
		}
		for _, lsp := range lsps {
			// find ovn_cluster_router programmatically, just in case
			// there should be only one router port here, ovn_cluster_router
			if lsp.Type == "router" {
				lr, _, err := findRouterForRouterPort(lsp.Options["router-port"].(string), client)
				if err == nil {
					g.Edge(nl.GetNode(lr.Name), nl.GetNode(ls.Name))
				}
			} else {
				g.Edge(nl.GetNode(ls.Name), nl.GetNode(lsp.Name))
			}
		}
	}

	return g.String(), nil
}
