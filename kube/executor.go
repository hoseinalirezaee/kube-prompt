package kube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
	"k8s.io/client-go/kubernetes"
)

type CommandRunner func(input string, cmd *exec.Cmd) error

var kubernetesClientFactory = newKubernetesClient
var kubePromptExecutable = os.Executable

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

	rewritten, err := rewritePodOwnerShortcut(context.Background(), s, kubeconfig, proxyURL, session.Namespace())
	if err != nil {
		fmt.Printf("Got error: %s\n", err.Error())
		return
	}
	rewritten, err = rewriteSecretDecodePipeline(rewritten)
	if err != nil {
		fmt.Printf("Got error: %s\n", err.Error())
		return
	}

	cmd := kubectlCommand(rewritten, kubeconfig, session.Namespace(), proxyURL)
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

func rewritePodOwnerShortcut(ctx context.Context, input, kubeconfig, proxyURL, namespace string) (string, error) {
	beforePipe, afterPipe := splitCommandBeforePipe(input)
	fields := strings.Fields(beforePipe)
	if len(fields) < 3 || fields[0] != "get" || !isPodResourceToken(fields[1]) {
		return input, nil
	}

	ownerIndex := -1
	for i := 2; i < len(fields); i++ {
		if _, _, ok := parsePodOwnerToken(fields[i]); !ok {
			continue
		}
		if ownerIndex != -1 {
			return "", fmt.Errorf("only one pod owner shortcut is supported per command")
		}
		ownerIndex = i
	}
	if ownerIndex == -1 {
		return input, nil
	}
	if hasSelectorFlag(fields) {
		return "", fmt.Errorf("pod owner shortcuts cannot be combined with --selector or -l")
	}
	filteredArgs, _ := excludeOptions(fields)
	if len(filteredArgs) != 3 || filteredArgs[2] != fields[ownerIndex] {
		return "", fmt.Errorf("pod owner shortcuts cannot be combined with additional resource names")
	}

	resolvedNamespace, allNamespaces := commandNamespace(fields, namespace)
	if allNamespaces {
		return "", fmt.Errorf("pod owner shortcuts do not support --all-namespaces")
	}
	if resolvedNamespace == "" {
		return "", fmt.Errorf("pod owner shortcuts require a namespace, use /namespace or --namespace")
	}

	client, err := kubernetesClientFactory(kubeconfig, proxyURL)
	if err != nil {
		return "", err
	}
	owner, err := resolvePodOwnerSelector(ctx, client, resolvedNamespace, fields[ownerIndex])
	if err != nil {
		return "", err
	}

	rewrittenFields := make([]string, 0, len(fields)+1)
	rewrittenFields = append(rewrittenFields, fields[:ownerIndex]...)
	rewrittenFields = append(rewrittenFields, "-l", owner.Selector)
	rewrittenFields = append(rewrittenFields, fields[ownerIndex+1:]...)

	rewritten := strings.Join(rewrittenFields, " ")
	if afterPipe == "" {
		return rewritten, nil
	}
	return rewritten + " " + strings.TrimLeft(afterPipe, " "), nil
}

func splitCommandBeforePipe(input string) (beforePipe, afterPipe string) {
	before, after, ok := splitFirstShellPipe(input)
	if !ok {
		return input, ""
	}
	return strings.TrimSpace(before), "|" + after
}

func rewriteSecretDecodePipeline(input string) (string, error) {
	segments := shellPipeSegments(input)
	if len(segments) < 2 {
		return input, nil
	}

	decoderIndex := -1
	for i := 1; i < len(segments); i++ {
		fields := strings.Fields(strings.TrimSpace(segments[i]))
		if len(fields) > 0 && fields[0] == SecretDecodeCommand {
			if decoderIndex != -1 {
				return "", fmt.Errorf("%s can only be used once in a pipeline", SecretDecodeCommand)
			}
			decoderIndex = i
		}
	}
	if decoderIndex == -1 {
		return input, nil
	}
	if decoderIndex != len(segments)-1 {
		return "", fmt.Errorf("%s must be the final pipe command", SecretDecodeCommand)
	}
	if strings.TrimSpace(segments[decoderIndex]) != SecretDecodeCommand {
		return "", fmt.Errorf("usage: get secret NAME | %s", SecretDecodeCommand)
	}
	if len(segments) != 2 {
		return "", fmt.Errorf("%s only supports direct pipelines from get secret NAME", SecretDecodeCommand)
	}

	left := strings.TrimSpace(segments[0])
	fields := strings.Fields(left)
	if _, allNamespaces := commandNamespace(fields, ""); allNamespaces {
		return "", fmt.Errorf("%s does not support --all-namespaces", SecretDecodeCommand)
	}
	filtered, skipNext := excludeOptions(fields)
	if skipNext {
		return "", fmt.Errorf("usage: get secret NAME | %s", SecretDecodeCommand)
	}
	if err := validateSecretDecodeGetArgs(filtered); err != nil {
		return "", err
	}

	executable, err := kubePromptExecutable()
	if err != nil {
		return "", fmt.Errorf("failed to resolve kube-prompt executable: %w", err)
	}
	return left + " -o json | " + shellQuote(executable) + " " + SecretDecodeInternalFlag, nil
}

func validateSecretDecodeGetArgs(args []string) error {
	if len(args) == 2 && args[0] == "get" {
		if name, ok := secretNameFromResourcePath(args[1]); ok && name != "" {
			return nil
		}
	}
	if len(args) == 3 && args[0] == "get" && isSecretResourceToken(args[1]) && args[2] != "" {
		return nil
	}
	if len(args) > 3 && args[0] == "get" && isSecretResourceToken(args[1]) {
		return fmt.Errorf("%s only supports one named Secret", SecretDecodeCommand)
	}
	return fmt.Errorf("usage: get secret NAME | %s", SecretDecodeCommand)
}

func secretNameFromResourcePath(resource string) (string, bool) {
	for _, prefix := range []string{"secret/", "secrets/"} {
		if strings.HasPrefix(resource, prefix) {
			return strings.TrimPrefix(resource, prefix), true
		}
	}
	return "", false
}

func isSecretResourceToken(token string) bool {
	switch token {
	case "secret", "secrets":
		return true
	default:
		return false
	}
}

func isPodResourceToken(token string) bool {
	switch token {
	case "po", "pod", "pods":
		return true
	default:
		return false
	}
}

func hasSelectorFlag(fields []string) bool {
	for i := range fields {
		field := fields[i]
		switch {
		case field == "-l" || field == "--selector":
			return true
		case strings.HasPrefix(field, "-l="):
			return true
		case strings.HasPrefix(field, "--selector="):
			return true
		}
	}
	return false
}

func commandNamespace(fields []string, fallback string) (string, bool) {
	namespace := strings.TrimSpace(fallback)

	for i := 0; i < len(fields); i++ {
		field := fields[i]
		switch {
		case field == "--namespace" || field == "-n":
			if i+1 < len(fields) {
				return fields[i+1], false
			}
		case field == "--all-namespaces" || field == "-A":
			return "", true
		case strings.HasPrefix(field, "--namespace="):
			return strings.TrimPrefix(field, "--namespace="), false
		case strings.HasPrefix(field, "--all-namespaces="):
			value := strings.TrimPrefix(field, "--all-namespaces=")
			if value == "" || strings.EqualFold(value, "true") {
				return "", true
			}
		case strings.HasPrefix(field, "-n="):
			return strings.TrimPrefix(field, "-n="), false
		case strings.HasPrefix(field, "-n") && len(field) > len("-n"):
			return strings.TrimPrefix(field, "-n"), false
		}
	}
	return namespace, false
}

func newKubernetesClient(kubeconfig, proxyURL string) (kubernetes.Interface, error) {
	config, err := newRESTConfig(kubeconfig, proxyURL)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
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
