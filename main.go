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

	ovnplot "github.com/andreaskaris/ovnkube-plot/pkg/ovnplot"
)

const (
	tabWriterMinWidth = 1
	tabWriterTabWidth = 8
	tabWriterPadding  = 2
	tabWriterPadC     = ' '
	tabWriterFlags    = 0

	// CustomDBCheckAppHelpTemplate helps in grouping options to ovnkube.
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

var format string //nolint: gochecknoglobals // skipping
var filter string //nolint: gochecknoglobals // skiping
var mode string   //nolint: gochecknoglobals // skipping

// OVNK8sFeatureFlags capture OVN-Kubernetes feature related options.
var customFlags = []cli.Flag{ //nolint: gochecknoglobals // skipping
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
	&cli.StringFlag{
		Name:        "mode",
		Usage:       "The mode to use ('auto', 'ovn-kubernetes', or 'ovn')",
		Value:       "auto",
		Destination: &mode,
	},
}

func getFlagsByCategory() map[string][]cli.Flag {
	m := map[string][]cli.Flag{}
	m["OVN Northbound DB Options"] = config.OvnNBFlags
	m["OVN Kubeplot Options"] = customFlags
	return m
}

// borrowed from cli packages' printHelpCustom().
func printOvnKubePlotHelp(out io.Writer, templ string, data interface{}, customFunc map[string]interface{}) {
	funcMap := template.FuncMap{
		"join":               strings.Join,
		"upper":              strings.ToUpper,
		"getFlagsByCategory": getFlagsByCategory,
	}
	for key, value := range customFunc {
		funcMap[key] = value
	}

	w := tabwriter.NewWriter(out, tabWriterMinWidth, tabWriterTabWidth, tabWriterPadding, tabWriterPadC, tabWriterFlags)
	t := template.Must(template.New("help").Funcs(funcMap).Parse(templ))
	err := t.Execute(w, data)
	if err == nil {
		_ = w.Flush()
	}
}

func main() {
	cli.HelpPrinterCustom = printOvnKubePlotHelp //nolint:reassign // skipping
	c := cli.NewApp()
	c.Name = "ovnkube-plot"
	c.Usage = "plot ovnkube network in a human readable way"
	c.Version = "0.0.1"
	c.CustomAppHelpTemplate = CustomDBCheckAppHelpTemplate
	c.Flags = config.GetFlags(customFlags)

	c.Action = runOvnKubePlot

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

	var plotMode ovnplot.Mode
	switch mode {
	case "ovn-kubernetes":
		plotMode = ovnplot.ModeOVNKube
	case "ovn":
		plotMode = ovnplot.ModeOVN
	case "auto":
		plotMode = ovnplot.ModeAuto
	default:
		return fmt.Errorf("unsupported mode %q (supported modes: 'ovn-kubernetes', 'ovn', 'auto')", mode)
	}

	var output string
	plot, err := ovnplot.New(ovnNBClient, plotMode, filter)
	if err != nil {
		return err
	}
	if format == "detailed" {
		output, err = plot.DetailedPlot()
	} else {
		output, err = plot.CompactPlot()
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(os.Stdout, output+"\n")
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}
