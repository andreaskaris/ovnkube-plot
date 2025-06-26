package ovnplot

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	goovn "github.com/ebay/go-ovn"
	"github.com/emicklei/dot"
)

type Mode int

const (
	_ Mode = iota
	ModeOVN
	ModeOVNKube
	ModeAuto
)

const (
	lspTypeRouter = "router"

	// Magic numbers for graph levels.
	ovnKubeLevels = 10
	ovnLevels     = 4
)

// OVNPlot traverses the OVN database. In order to do so, it must connect to the OVN NB client which is stored in client.
type OVNPlot struct {
	client          goovn.Client
	mode            Mode
	filter          string
	lspCache        map[string][]*goovn.LogicalSwitchPort // Cache LSPList results and categorize switches by type.
	regularSwitches []*goovn.LogicalSwitch
	joinSwitches    []*goovn.LogicalSwitch
	extSwitches     []*goovn.LogicalSwitch
	routers         []*goovn.LogicalRouter // Cache routers connected to switches
}

func New(ovnNBClient goovn.Client, mode Mode, filter string) (*OVNPlot, error) {
	lspCache := make(map[string][]*goovn.LogicalSwitchPort)
	var regularSwitches []*goovn.LogicalSwitch
	var joinSwitches []*goovn.LogicalSwitch
	var extSwitches []*goovn.LogicalSwitch
	var routers []*goovn.LogicalRouter

	op := &OVNPlot{
		ovnNBClient, mode, filter, lspCache, regularSwitches, joinSwitches, extSwitches, routers,
	}
	if err := op.setAutoMode(); err != nil {
		return nil, err
	}
	if err := op.buildLSCache(); err != nil {
		return nil, err
	}
	if err := op.buildRouterCache(); err != nil {
		return nil, err
	}
	return op, nil
}

// DetailedPlot creates a detailed view of this OVN database.
func (op *OVNPlot) DetailedPlot() (string, error) {
	g := op.initializeGraph("4.0")

	// Draw all subgraphs for routers using cached routers
	routerPorts := make(map[string]dot.Node)
	for _, lr := range op.routers {
		_, rps, err := op.drawRouter(g, lr.Name)
		if err != nil {
			return "", err
		}
		maps.Insert(routerPorts, maps.All(rps))
	}

	// Draw all subgraphs for switches using cached data
	for _, ls := range op.getAllSwitches() {
		_, _, err := op.drawSwitch(g, ls.Name, routerPorts)
		if err != nil {
			return "", err
		}
	}

	return g.String(), nil
}

// CompactPlot draws a compact plot of the OVN Northbound database.
// Ideal for an overview of more complex systems as it provides a cleaner design.
func (op *OVNPlot) CompactPlot() (string, error) { //nolint:gocognit // Complex business logic for processing different switch types
	g := op.initializeGraph("2.0")
	nl := NewNodeList(g)

	// First, draw levels - this will help keep the document formatted. 10 levels for OVNKUBE, 4 levels for OVN.
	if op.mode == ModeOVNKube {
		nl.DrawLevels(ovnKubeLevels)
	} else {
		nl.DrawLevels(ovnLevels)
	}

	// "1", "2", "3" and "4" - Process regular switches and routers that are attached to them.
	for _, ls := range op.regularSwitches {
		lsps := op.lspCache[ls.Name]
		for _, lsp := range lsps {
			if lsp.Type == lspTypeRouter {
				rpName, ok := lsp.Options["router-port"].(string)
				if !ok {
					return "", fmt.Errorf("router-port option not found for lsp %s in switch %s", lsp.Name, ls.Name)
				}
				lr, lrp, err := op.findCachedRouterForRouterPort(rpName)
				if err != nil {
					return "", fmt.Errorf("cannot find router for router port, ls: %v, lsp: %v, err: %w", ls, lsp, err)
				}
				if lr == nil || lrp == nil {
					continue
				}
				label := strings.Join(lrp.Networks, ";")
				spacerName := ls.Name + "spacer1" + lr.Name
				nl.GetSwitch(ls.Name).Edge(nl.GetInvisible(spacerName)).Label(label).Edge(nl.GetRouter(lr.Name))
			} else {
				g.Edge(nl.GetLeaf(lsp.Name), nl.GetSwitch(ls.Name))
			}
		}
	}

	// Everything below for ovnk only.
	// "5" and "6" and "7" - Process join switches
	for _, ls := range op.joinSwitches {
		lsps := op.lspCache[ls.Name]
		for _, lsp := range lsps {
			if lsp.Type == lspTypeRouter { //nolint:nestif // Complex router processing logic
				rpName, ok := lsp.Options["router-port"].(string)
				if !ok {
					return "", fmt.Errorf("router-port option not found for lsp %s in switch %s", lsp.Name, ls.Name)
				}
				lr, lrp, err := op.findCachedRouterForRouterPort(rpName)
				if err != nil {
					return "", fmt.Errorf("cannot find router for router port, ls: %v, lsp: %v, err: %w", ls, lsp, err)
				}
				if lr == nil || lrp == nil {
					continue
				}
				if lr.Name == "ovn_cluster_router" {
					label := strings.Join(lrp.Networks, ";")
					spacerName := "ovn_cluster_router" + ls.Name + "spacer1"
					nl.GetRouter(lr.Name).Edge(nl.GetInvisible(spacerName)).Label(label).Edge(nl.GetSwitch(ls.Name))
				} else if matchesRegex(op.filter, lr.Name) {
					label := strings.Join(lrp.Networks, ";")
					nl.GetSwitch(ls.Name).Edge(nl.GetInvisible(lr.Name + "spacer1")).Edge(nl.GetRouter(lr.Name)).Label(label)
				}
			} else {
				g.Edge(nl.GetSwitch(ls.Name), nl.GetLeaf(lsp.Name))
			}
		}
	}

	// "8" and "9" and "10", process ext_* switches.
	for _, ls := range op.extSwitches {
		lsps := op.lspCache[ls.Name]
		for _, lsp := range lsps {
			// find ovn_cluster_router programmatically, just in case
			// there should be only one router port here, ovn_cluster_router
			if lsp.Type == lspTypeRouter {
				rpName, ok := lsp.Options["router-port"].(string)
				if !ok {
					return "", fmt.Errorf("router-port option not found for lsp %s in switch %s", lsp.Name, ls.Name)
				}
				lr, _, err := op.findCachedRouterForRouterPort(rpName)
				if err == nil && lr != nil {
					g.Edge(nl.GetRouter(lr.Name), nl.GetSwitch(ls.Name))
				}
			} else {
				g.Edge(nl.GetSwitch(ls.Name), nl.GetLeaf(lsp.Name))
			}
		}
	}

	return g.String(), nil
}

// findRouterForRouterPort is a helper function to retrieve both the LogicalRouter and the LogicalRouterPort that belong
// to string "routerPortName".
func (op *OVNPlot) findRouterForRouterPort(
	routerPortName string,
) (*goovn.LogicalRouter, *goovn.LogicalRouterPort, error) {
	lrs, lrErr := op.client.LRList()
	if lrErr != nil {
		return nil, nil, lrErr
	}
	for _, lr := range lrs {
		lrps, lrsErr := op.client.LRPList(lr.Name)
		if lrsErr != nil {
			return nil, nil, lrsErr
		}
		for _, lrp := range lrps {
			if lrp.Name == routerPortName {
				return lr, lrp, nil
			}
		}
	}
	return nil, nil, nil
}

// findCachedRouterForRouterPort finds router and router port from cached routers.
func (op *OVNPlot) findCachedRouterForRouterPort(
	routerPortName string,
) (*goovn.LogicalRouter, *goovn.LogicalRouterPort, error) {
	for _, lr := range op.routers {
		lrps, err := op.client.LRPList(lr.Name)
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

// buildLSCache builds the cache of logical switches.
func (op *OVNPlot) buildLSCache() error {
	// Get the OVN LogicalSwitch table content.
	lss, lsErr := op.client.LSList()
	if lsErr != nil {
		return lsErr
	}

	for _, ls := range lss {
		lsps, lspErr := op.client.LSPList(ls.Name)
		if lspErr != nil {
			return lspErr
		}
		op.lspCache[ls.Name] = lsps

		// Categorize switches - collect all switches as regular switches if MODE_OVN is forced.
		if op.mode == ModeOVN {
			if matchesRegex(op.filter, ls.Name) {
				op.regularSwitches = append(op.regularSwitches, ls)
			}
			continue
		}

		// Otherwise, use this logic for MOVE_OVNKUBE.
		switch {
		case !matchesRegex("^join.*|^node_local_switch$|^ext_.*", ls.Name) && matchesRegex(op.filter, ls.Name):
			op.regularSwitches = append(op.regularSwitches, ls)
		case matchesRegex("^join.*", ls.Name) && matchesRegex("^join$|"+op.filter, ls.Name):
			op.joinSwitches = append(op.joinSwitches, ls)
		case matchesRegex("^ext_.*", ls.Name) && matchesRegex(op.filter, ls.Name):
			op.extSwitches = append(op.extSwitches, ls)
		case matchesRegex("^node_local_switch$", ls.Name):
			// The "node_local_switch" is a special case, but for graphing it behaves the same as the extSwitches.
			op.extSwitches = append(op.extSwitches, ls)
		}
	}

	return nil
}

// buildRouterCache builds the cache of routers connected to switches.
func (op *OVNPlot) buildRouterCache() error { //nolint:gocognit // Complex logic for categorizing and caching routers from switches
	routerMap := make(map[string]*goovn.LogicalRouter)

	for _, ls := range op.getAllSwitches() {
		lsps := op.lspCache[ls.Name]
		for _, lsp := range lsps {
			if lsp.Type == lspTypeRouter { //nolint:nestif // Complex router processing logic
				rpName, ok := lsp.Options["router-port"].(string)
				if !ok {
					return fmt.Errorf("router-port option not found for lsp %s", lsp.Name)
				}
				logicalRouter, _, err := op.findRouterForRouterPort(rpName)
				if err != nil {
					return fmt.Errorf("cannot find router for router port, ls: %v, lsp: %v, err: %w", ls, lsp, err)
				}
				if logicalRouter != nil {
					shouldInclude := matchesRegex(op.filter, logicalRouter.Name)
					if op.mode == ModeOVNKube && logicalRouter.Name == "ovn_cluster_router" {
						shouldInclude = true
					}
					if shouldInclude {
						routerMap[logicalRouter.Name] = logicalRouter
					}
				}
			}
		}
	}

	op.routers = slices.Collect(maps.Values(routerMap))
	return nil
}

func (op *OVNPlot) setAutoMode() error {
	if op.mode != ModeAuto {
		return nil
	}

	lss, err := op.client.LSList()
	if err != nil {
		return err
	}
	for _, ls := range lss {
		if matchesRegex("^join.*", ls.Name) && matchesRegex("^join$|"+op.filter, ls.Name) {
			op.mode = ModeOVNKube
			return nil
		}
	}
	op.mode = ModeOVN
	return nil
}

// Draw every switch and router as a subgraph. Add interfaces and routes as individual nodes under the subgraphs.
// Then, connect all interfaces to each other, with a single connection between pairs
// Ideal when using the filter expression and when filtering for a very restricted number of nodes.
func (op *OVNPlot) initializeGraph(penwidth string) *dot.Graph {
	g := dot.NewGraph(dot.Directed)
	g.Attr("rankdir", "LR")
	g.EdgeInitializer(func(e dot.Edge) {
		e.Attr("fontname", "arial")
		e.Attr("fontsize", "9")
		e.Attr("penwidth", penwidth)
		e.Attr("arrowhead", "none")
	})
	return g
}

func matchesRegex(pattern, name string) bool {
	matched, _ := regexp.MatchString(pattern, name)
	return matched
}

func (op *OVNPlot) drawRouter(g *dot.Graph, lrName string) (*dot.Graph, map[string]dot.Node, error) {
	var router *dot.Graph
	routerPorts := make(map[string]dot.Node)
	router = g.Subgraph(lrName, dot.ClusterOption{})
	router.Attr("style", "filled")
	router.Attr("color", "0.7 0.7 1.0")
	lrps, err := op.client.LRPList(lrName)
	if err != nil {
		return router, routerPorts, err
	}
	staticRoutes, err := op.client.LRSRList(lrName)
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
		router.Node("routes-"+lrName).Attr("shape", "box").Attr("style", "filled").Attr("label", dot.HTML(routes))
	}
	for _, lrp := range lrps {
		routerPorts[lrp.Name] = router.Node(lrp.Name)
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
	return router, routerPorts, nil
}

func (op *OVNPlot) drawSwitch(
	g *dot.Graph, lsName string, routerPorts map[string]dot.Node) (*dot.Graph, map[string]dot.Node, error) {
	var sw *dot.Graph
	swPorts := make(map[string]dot.Node)

	sw = g.Subgraph(lsName, dot.ClusterOption{})
	sw.Attr("style", "filled")
	sw.Attr("color", "0.4 1.0 0.6")
	lsps := op.lspCache[lsName]
	for _, lsp := range lsps {
		swPorts[lsp.Name] = sw.Node(lsp.Name)
		swPorts[lsp.Name].Attr("shape", "box")
		swPorts[lsp.Name].Attr("style", "filled")
		swPorts[lsp.Name].Attr("color", "white")
		if lsp.Type == lspTypeRouter {
			rpName, ok := lsp.Options["router-port"].(string)
			if !ok {
				return nil, nil, fmt.Errorf("router-port option not found for lsp %s", lsp.Name)
			}
			if routerPort, exists := routerPorts[rpName]; exists {
				g.Edge(swPorts[lsp.Name], routerPort)
			}
		}
	}

	return sw, swPorts, nil
}

func (op *OVNPlot) getAllSwitches() []*goovn.LogicalSwitch {
	allSwitches := make([]*goovn.LogicalSwitch, 0)
	allSwitches = append(allSwitches, op.regularSwitches...)
	allSwitches = append(allSwitches, op.joinSwitches...)
	allSwitches = append(allSwitches, op.extSwitches...)
	return allSwitches
}
