package kube

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestKubectlCommandSetsKubeconfigEnv(t *testing.T) {
	cmd := kubectlCommand("get pods", "/tmp/kubeconfig", "", "")

	if cmd.Env == nil {
		t.Fatal("expected command environment to be set")
	}
	if !hasEnv(cmd.Env, "KUBECONFIG=/tmp/kubeconfig") {
		t.Fatalf("expected KUBECONFIG in command environment, got %v", cmd.Env)
	}
}

func TestKubectlEnvKeepsExistingProxyEnvWithoutExplicitProxy(t *testing.T) {
	env := kubectlEnv([]string{
		"HTTP_PROXY=http://old.example",
		"NO_PROXY=example.com",
	}, "/tmp/kubeconfig", "")

	if !hasEnv(env, "HTTP_PROXY=http://old.example") {
		t.Fatalf("expected existing HTTP_PROXY to remain, got %v", env)
	}
	if !hasEnv(env, "NO_PROXY=example.com") {
		t.Fatalf("expected existing NO_PROXY to remain, got %v", env)
	}
}

func TestKubectlCommandLeavesDefaultEnvWithoutKubeconfig(t *testing.T) {
	cmd := kubectlCommand("get pods", "", "", "")

	if cmd.Env != nil {
		t.Fatalf("expected nil command environment, got %v", cmd.Env)
	}
}

func TestKubectlCommandSetsProxyEnv(t *testing.T) {
	cmd := kubectlCommand("get pods", "/tmp/kubeconfig", "", "socks5h://proxy.example:1080")

	for _, want := range []string{
		"KUBECONFIG=/tmp/kubeconfig",
		"HTTP_PROXY=socks5h://proxy.example:1080",
		"HTTPS_PROXY=socks5h://proxy.example:1080",
		"ALL_PROXY=socks5h://proxy.example:1080",
		"http_proxy=socks5h://proxy.example:1080",
		"https_proxy=socks5h://proxy.example:1080",
		"all_proxy=socks5h://proxy.example:1080",
	} {
		if !hasEnv(cmd.Env, want) {
			t.Fatalf("expected %s in command environment, got %v", want, cmd.Env)
		}
	}
}

func TestKubectlEnvProxyOverridesExistingProxyEnv(t *testing.T) {
	env := kubectlEnv([]string{
		"PATH=/bin",
		"HTTP_PROXY=http://old.example",
		"https_proxy=http://old.example",
		"NO_PROXY=example.com",
		"no_proxy=example.com",
	}, "", "https://proxy.example:8443")

	for _, unwanted := range []string{
		"HTTP_PROXY=http://old.example",
		"https_proxy=http://old.example",
		"NO_PROXY=example.com",
		"no_proxy=example.com",
	} {
		if hasEnv(env, unwanted) {
			t.Fatalf("did not expect %s in command environment, got %v", unwanted, env)
		}
	}
	for _, want := range []string{
		"PATH=/bin",
		"HTTP_PROXY=https://proxy.example:8443",
		"https_proxy=https://proxy.example:8443",
	} {
		if !hasEnv(env, want) {
			t.Fatalf("expected %s in command environment, got %v", want, env)
		}
	}
}

func TestKubectlCommandLineInjectsNamespace(t *testing.T) {
	if got, want := kubectlCommandLine("get pods", "apps"), "kubectl --namespace 'apps' get pods"; got != want {
		t.Fatalf("expected command line %q, got %q", want, got)
	}
}

func TestKubectlCommandLineDoesNotInjectWhenNamespaceProvided(t *testing.T) {
	tests := []string{
		"get pods --namespace kube-system",
		"get pods --namespace=kube-system",
		"get pods -n kube-system",
		"get pods -n=kube-system",
		"get pods -nkube-system",
		"get pods --all-namespaces",
		"get pods --all-namespaces=true",
		"get pods -A",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if got, want := kubectlCommandLine(input, "apps"), "kubectl "+input; got != want {
				t.Fatalf("expected command line %q, got %q", want, got)
			}
		})
	}
}

func TestKubectlCommandLineOnlyChecksNamespaceBeforePipe(t *testing.T) {
	input := "get pods | grep --namespace"

	if got, want := kubectlCommandLine(input, "apps"), "kubectl --namespace 'apps' "+input; got != want {
		t.Fatalf("expected command line %q, got %q", want, got)
	}
}

func TestRunSessionNamespaceCommand(t *testing.T) {
	session := NewSessionState("default")
	var out bytes.Buffer

	result := runSessionCommand("/namespace apps", session, &out)

	if !result.handled || result.exit {
		t.Fatalf("expected handled non-exit result, got %#v", result)
	}
	if got := session.Namespace(); got != "apps" {
		t.Fatalf("expected namespace apps, got %q", got)
	}
	if !strings.Contains(out.String(), "namespace: apps") {
		t.Fatalf("expected namespace output, got %q", out.String())
	}
}

func TestRunSessionNamespaceCommandPrintsCurrentNamespace(t *testing.T) {
	session := NewSessionState("")
	var out bytes.Buffer

	result := runSessionCommand("/namespace", session, &out)

	if !result.handled || result.exit {
		t.Fatalf("expected handled non-exit result, got %#v", result)
	}
	if got := strings.TrimSpace(out.String()); got != "namespace: -" {
		t.Fatalf("expected current namespace output, got %q", got)
	}
}

func TestRunSessionCommandExit(t *testing.T) {
	var out bytes.Buffer

	result := runSessionCommand("/exit", NewSessionState(""), &out)

	if !result.handled || !result.exit {
		t.Fatalf("expected handled exit result, got %#v", result)
	}
	if got := strings.TrimSpace(out.String()); got != "Bye!" {
		t.Fatalf("expected bye output, got %q", got)
	}
}

func TestNewExecutorWithRunnerUsesCustomRunner(t *testing.T) {
	session := NewSessionState("apps")
	var gotInput string
	var gotCommand string
	executor := NewExecutorWithRunner("/tmp/kubeconfig", "", session, func(input string, cmd *exec.Cmd) error {
		gotInput = input
		gotCommand = strings.Join(cmd.Args, " ")
		return nil
	})

	executor("get pods")

	if gotInput != "get pods" {
		t.Fatalf("expected runner input %q, got %q", "get pods", gotInput)
	}
	if !strings.Contains(gotCommand, "kubectl --namespace 'apps' get pods") {
		t.Fatalf("expected kubectl command with namespace, got %q", gotCommand)
	}
}

func hasEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
