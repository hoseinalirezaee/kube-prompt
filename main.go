package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/c-bata/go-prompt"
	"github.com/c-bata/go-prompt/completer"
	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
	"github.com/hoseinalirezaee/kube-prompt/kube"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	version  string
	revision string
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type cliConfig struct {
	kubeconfig       string
	kubeconfigStatus string
	version          bool
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, ok := parseCLI(args, stdout, stderr)
	if !ok {
		return 2
	}
	if cfg == nil {
		return 0
	}
	resolved, err := requireKubeconfig(*cfg, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return runPrompt(context.TODO(), resolved, stdout, stderr)
}

var errKubeconfigRequired = errors.New("kubeconfig is required: set KUBECONFIG or pass --kubeconfig PATH")

func requireKubeconfig(cfg cliConfig, getenv func(string) string) (cliConfig, error) {
	if cfg.kubeconfig != "" {
		cfg.kubeconfigStatus = cfg.kubeconfig
		return cfg, nil
	}
	if envKubeconfig := getenv("KUBECONFIG"); envKubeconfig != "" {
		cfg.kubeconfigStatus = envKubeconfig
		return cfg, nil
	}
	return cfg, errKubeconfigRequired
}

func parseCLI(args []string, stdout, stderr io.Writer) (*cliConfig, bool) {
	var (
		cfg  cliConfig
		help bool
	)

	fs := flag.NewFlagSet("kube-prompt", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&help, "h", false, "show help")
	fs.BoolVar(&help, "help", false, "show help")
	fs.BoolVar(&cfg.version, "v", false, "show version")
	fs.BoolVar(&cfg.version, "version", false, "show version")
	fs.StringVar(&cfg.kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return nil, false
	}

	if help {
		printUsage(stdout)
		return nil, true
	}
	if cfg.version {
		fmt.Fprintln(stdout, versionString())
		return nil, true
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", fs.Arg(0))
		printUsage(stderr)
		return nil, false
	}
	return &cfg, true
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: kube-prompt [flags]

kube-prompt is an interactive Kubernetes client. Inside the prompt, type
kubectl-style commands without the kubectl prefix.

Flags:
  -h, --help              Show help and exit.
  -v, --version           Show version and exit.
      --kubeconfig PATH   Path to the kubeconfig file to use for this session.

Examples:
  get pods
  describe pod <name>
  get pods | grep web
  exit
`)
}

func versionString() string {
	v := version
	if v == "" {
		v = "dev"
	}
	r := revision
	if r == "" {
		r = "unknown"
	}
	return fmt.Sprintf("kube-prompt %s (rev-%s)", v, r)
}

func runPrompt(ctx context.Context, cfg cliConfig, stdout, stderr io.Writer) int {
	c, err := kube.NewCompleter(ctx, cfg.kubeconfig)
	if err != nil {
		fmt.Fprintln(stderr, "error", err)
		return 1
	}

	defer debug.Teardown()
	statusWriter := newStatusLineWriter(prompt.NewStdoutWriter(), kubeconfigStatusLine(cfg))
	statusWriter.Attach()
	defer statusWriter.Close()

	fmt.Fprintln(stdout, versionString())
	fmt.Fprintln(stdout, "Please use `exit` or `Ctrl-D` to exit this program.")
	defer fmt.Fprintln(stdout, "Bye!")

	promptOptions := []prompt.Option{
		prompt.OptionWriter(statusWriter),
		prompt.OptionTitle("kube-prompt: interactive kubernetes client"),
		prompt.OptionPrefix(">>> "),
		prompt.OptionInputTextColor(prompt.Yellow),
		prompt.OptionCompletionWordSeparator(completer.FilePathCompletionSeparator),
	}
	if statusWriter.attached {
		promptOptions = append(promptOptions, prompt.OptionParser(newStatusLineParser(prompt.NewStandardInputParser())))
	}

	p := prompt.New(
		kube.NewExecutor(cfg.kubeconfig),
		c.Complete,
		promptOptions...,
	)
	p.Run()
	return 0
}

func kubeconfigStatusLine(cfg cliConfig) string {
	return " kube-prompt | kubeconfig: " + cfg.kubeconfigStatus + " "
}
