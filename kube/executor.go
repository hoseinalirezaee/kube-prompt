package kube

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
)

type CommandRunner func(input string, cmd *exec.Cmd) error

func NewExecutor(kubeconfig, proxyURL string, session *SessionState) func(string) {
	return NewExecutorWithRunner(kubeconfig, proxyURL, session, directCommandRunner)
}

func NewExecutorWithRunner(kubeconfig, proxyURL string, session *SessionState, runner CommandRunner) func(string) {
	return func(s string) {
		execute(s, kubeconfig, proxyURL, session, runner)
	}
}

func Executor(s string) {
	execute(s, "", "", NewSessionState(""), directCommandRunner)
}

func execute(s, kubeconfig, proxyURL string, session *SessionState, runner CommandRunner) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	if result := runSessionCommand(s, session, os.Stdout); result.handled {
		if result.exit {
			os.Exit(0)
		}
		return
	}

	cmd := kubectlCommand(s, kubeconfig, session.Namespace(), proxyURL)
	if runner == nil {
		runner = directCommandRunner
	}
	if err := runner(s, cmd); err != nil {
		fmt.Printf("Got error: %s\n", err.Error())
	}
}

func directCommandRunner(_ string, cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ExecuteAndGetResult(s string) string {
	return ExecuteAndGetResultWithKubeconfig(s, "")
}

func ExecuteAndGetResultWithKubeconfig(s, kubeconfig string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		debug.Log("you need to pass the something arguments")
		return ""
	}

	out := &bytes.Buffer{}
	cmd := kubectlCommand(s, kubeconfig, "", "")
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		debug.Log(err.Error())
		return ""
	}
	r := string(out.Bytes())
	return r
}

type sessionCommandResult struct {
	handled bool
	exit    bool
}

func runSessionCommand(s string, session *SessionState, out io.Writer) sessionCommandResult {
	switch s {
	case "quit", "exit", "/quit", "/exit":
		fmt.Fprintln(out, "Bye!")
		return sessionCommandResult{handled: true, exit: true}
	}
	if !strings.HasPrefix(s, "/") {
		return sessionCommandResult{}
	}

	fields := strings.Fields(s)
	if len(fields) == 0 {
		return sessionCommandResult{handled: true}
	}
	switch fields[0] {
	case "/namespace":
		if len(fields) == 1 {
			namespace := session.Namespace()
			if namespace == "" {
				namespace = "-"
			}
			fmt.Fprintf(out, "namespace: %s\n", namespace)
			return sessionCommandResult{handled: true}
		}
		if len(fields) == 2 {
			session.SetNamespace(fields[1])
			fmt.Fprintf(out, "namespace: %s\n", session.Namespace())
			return sessionCommandResult{handled: true}
		}
		fmt.Fprintln(out, "usage: /namespace NAME")
	default:
		fmt.Fprintf(out, "unknown kube-prompt command: %s\n", fields[0])
	}
	return sessionCommandResult{handled: true}
}

func kubectlCommand(s, kubeconfig, namespace, proxyURL string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", kubectlCommandLine(s, namespace))
	if kubeconfig != "" || proxyURL != "" {
		cmd.Env = kubectlEnv(os.Environ(), kubeconfig, proxyURL)
	}
	return cmd
}

func kubectlEnv(base []string, kubeconfig, proxyURL string) []string {
	env := make([]string, 0, len(base)+8)
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if key == "KUBECONFIG" && kubeconfig != "" {
			continue
		}
		if proxyURL != "" && isProxyEnvKey(key) {
			continue
		}
		env = append(env, item)
	}

	if kubeconfig != "" {
		env = append(env, "KUBECONFIG="+kubeconfig)
	}
	if proxyURL != "" {
		env = append(env,
			"HTTP_PROXY="+proxyURL,
			"HTTPS_PROXY="+proxyURL,
			"ALL_PROXY="+proxyURL,
			"http_proxy="+proxyURL,
			"https_proxy="+proxyURL,
			"all_proxy="+proxyURL,
		)
	}
	return env
}

func isProxyEnvKey(key string) bool {
	switch key {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
		"http_proxy", "https_proxy", "all_proxy", "no_proxy":
		return true
	default:
		return false
	}
}

func kubectlCommandLine(s, namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" || commandDeclaresNamespace(s) {
		return "kubectl " + s
	}
	return "kubectl --namespace " + shellQuote(namespace) + " " + s
}

func commandDeclaresNamespace(s string) bool {
	beforePipe := s
	if pipe := strings.Index(beforePipe, "|"); pipe >= 0 {
		beforePipe = beforePipe[:pipe]
	}
	for _, field := range strings.Fields(beforePipe) {
		switch {
		case field == "--namespace" || field == "-n":
			return true
		case field == "--all-namespaces" || field == "-A":
			return true
		case strings.HasPrefix(field, "--namespace="):
			return true
		case strings.HasPrefix(field, "--all-namespaces="):
			return true
		case strings.HasPrefix(field, "-n="):
			return true
		case strings.HasPrefix(field, "-n") && len(field) > len("-n"):
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
