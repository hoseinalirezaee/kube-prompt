package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
	"github.com/hoseinalirezaee/kube-prompt/kube"
	"github.com/hoseinalirezaee/kube-prompt/prompt"
	"github.com/hoseinalirezaee/kube-prompt/prompt/completer"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	version  string
	revision string
)

var completionWordSeparator = completer.FilePathCompletionSeparator + "|"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type cliConfig struct {
	kubeconfig       string
	kubeconfigStatus string
	defaultNamespace string
	proxyURL         string
	proxyStatus      string
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
	resolved = resolveProxyStatus(resolved)
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
	fs.StringVar(&cfg.defaultNamespace, "default-namespace", "", "namespace to use when commands do not provide one")
	fs.StringVar(&cfg.proxyURL, "proxy", "", "proxy URL for Kubernetes API requests")
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
	if err := kube.ValidateProxyURL(cfg.proxyURL); err != nil {
		fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return nil, false
	}
	return &cfg, true
}

func resolveProxyStatus(cfg cliConfig) cliConfig {
	if cfg.proxyURL != "" {
		cfg.proxyStatus = kube.ProxyDisplayString(cfg.proxyURL)
		return cfg
	}
	proxyStatus, err := kube.KubeconfigProxyDisplayString(cfg.kubeconfig)
	if err == nil {
		cfg.proxyStatus = proxyStatus
	}
	return cfg
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: kube-prompt [flags]

kube-prompt is an interactive Kubernetes client. Inside the prompt, type
kubectl-style commands without the kubectl prefix.

Flags:
  -h, --help              Show help and exit.
  -v, --version           Show version and exit.
      --kubeconfig PATH   Path to the kubeconfig file to use for this session.
      --default-namespace NAME
                          Namespace to use when commands do not provide one.
      --proxy URL         Proxy URL for Kubernetes API requests.
                          Supports http, https, and socks5h.

Examples:
  get pods
  describe pod <name>
  /namespace production
  get pods | grep web
  /exit
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
	session := kube.NewSessionState("")
	c, err := kube.NewCompleter(ctx, cfg.kubeconfig, session, cfg.defaultNamespace, cfg.proxyURL)
	if err != nil {
		fmt.Fprintln(stderr, "error", err)
		return 1
	}
	defer c.Close()

	defer debug.Teardown()
	outputStatus := &outputModeStatus{}
	statusWriter := newDynamicStatusLineWriter(prompt.NewStdoutWriter(), func() string {
		if label := outputStatus.Label(); label != "" {
			return kubeconfigStatusLineWithMode(cfg, session, label)
		}
		return kubeconfigStatusLine(cfg, session) + outputStatus.Suffix()
	})
	statusWriter.Attach()
	defer statusWriter.Close()
	outputRunner := newManagedOutputRunner(statusWriter, outputStatus)
	defer outputRunner.Close()

	printStartupMessage(stdout)
	defer fmt.Fprintln(stdout, "Bye!")

	promptOptions := []prompt.Option{
		prompt.OptionWriter(statusWriter),
		prompt.OptionTitle("kube-prompt: interactive kubernetes client"),
		prompt.OptionPrefix(">>> "),
		prompt.OptionInputTextColor(prompt.Yellow),
		prompt.OptionCompletionWordSeparator(completionWordSeparator),
	}
	if statusWriter.attached {
		promptOptions = append(promptOptions, prompt.OptionParser(newStatusLineParser(prompt.NewStandardInputParser(), outputRunner.ShowHistory)))
	}

	executor := kube.NewExecutorWithRunner(
		cfg.kubeconfig,
		cfg.proxyURL,
		session,
		outputRunner.Run,
	)
	p := prompt.New(
		func(input string) {
			if outputRunner.HandlePromptCommand(input) {
				return
			}
			executor(input)
		},
		c.Complete,
		promptOptions...,
	)
	p.Run()
	return 0
}

func kubeconfigStatusLine(cfg cliConfig, session *kube.SessionState) string {
	return kubeconfigStatusLineWithMode(cfg, session, "")
}

func kubeconfigStatusLineWithMode(cfg cliConfig, session *kube.SessionState, mode string) string {
	namespace := session.Namespace()
	if namespace == "" {
		namespace = "-"
	}
	proxyStatus := cfg.proxyStatus
	if proxyStatus == "" {
		proxyStatus = "-"
	}
	status := " kube-prompt"
	if mode != "" {
		status += " | " + mode
	}
	return status + " | kubeconfig: " + cfg.kubeconfigStatus + " | namespace: " + namespace + " | proxy: " + proxyStatus + " "
}
