package main

import (
	"fmt"
	"io"
	"os"
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

func getFlagsByCategory() map[string][]cli.Flag {
	m := map[string][]cli.Flag{}
	m["OVN Northbound DB Options"] = config.OvnNBFlags
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
	c.Flags = config.GetFlags(nil)

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

	var lrs []*goovn.LogicalRouter
	lrs, err = ovnNBClient.LRList()
	if err != nil {
		return err
	}

	g := dot.NewGraph(dot.Directed)
	// g.Attr("splines", "false")
	g.Attr("rankdir", "LR")
	g.EdgeInitializer(func(e dot.Edge) {
		e.Attr("fontname", "arial")
		e.Attr("fontsize", "9")
		e.Attr("penwidth", "4.0")
		e.Attr("arrowhead", "none")
	})

	routers := map[string]*dot.Graph{}
	routerPorts := map[string]dot.Node{}
	for _, lr := range lrs {
		routers[lr.Name] = g.Subgraph(lr.Name, dot.ClusterOption{})
		routers[lr.Name].Attr("style", "filled")
		routers[lr.Name].Attr("color", "0.7 0.7 1.0")
		lrps, err := ovnNBClient.LRPList(lr.Name)
		if err != nil {
			return err
		}
		staticRoutes, err := ovnNBClient.LRSRList(lr.Name)
		if err == nil {
			routes := "<table BORDER='0' CELLBORDER='0' CELLSPACING='0' CELLPADDING='0'><tr><td>IPPrefix</td><td>Nexthop</td><td>OutputPort</td><td>Policy</td></tr>"
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
			routers[lr.Name].Node("routes-" + lr.Name).Attr("shape", "box").Attr("style", "filled").Attr("label", dot.HTML(routes))
		}
		for _, lrp := range(lrps) {
			routerPorts[lrp.Name] = routers[lr.Name].Node(lrp.Name)
			label := fmt.Sprintf("<table><tr><td>Name</td><td>Networks</td><td>MAC</td></tr>" +
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


	switches := map[string]*dot.Graph{}
	switchPorts := map[string]dot.Node{}
	var lss []*goovn.LogicalSwitch
	lss, err = ovnNBClient.LSList()
	if err != nil {
		return err
	}
	for _, ls := range lss {
		switches[ls.Name] = g.Subgraph(ls.Name, dot.ClusterOption{})
		switches[ls.Name].Attr("style", "filled")
		switches[ls.Name].Attr("color", "0.4 1.0 0.6")
		lsps, err := ovnNBClient.LSPList(ls.Name)
		if err != nil {
			return err
		}
		for _, lsp := range(lsps) {
			switchPorts[lsp.Name] = switches[ls.Name].Node(lsp.Name)
			switchPorts[lsp.Name].Attr("shape", "box")
			switchPorts[lsp.Name].Attr("style", "filled")
			switchPorts[lsp.Name].Attr("color", "white")
			// g.Edge(switches[ls.Name], switchPorts[lsp.Name])
			if lsp.Type == "router" {
				routerPortName := lsp.Options["router-port"].(string)
				g.Edge(switchPorts[lsp.Name], routerPorts[routerPortName])
			}
		}
	}

	fmt.Println(g.String())

	return nil
}
