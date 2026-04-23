package kube

import (
	"bytes"
	"strings"
	"testing"
)

func TestKubectlCommandSetsKubeconfigEnv(t *testing.T) {
	cmd := kubectlCommand("get pods", "/tmp/kubeconfig", "")

	if cmd.Env == nil {
		t.Fatal("expected command environment to be set")
	}
	if !hasEnv(cmd.Env, "KUBECONFIG=/tmp/kubeconfig") {
		t.Fatalf("expected KUBECONFIG in command environment, got %v", cmd.Env)
	}
}

func TestKubectlCommandLeavesDefaultEnvWithoutKubeconfig(t *testing.T) {
	cmd := kubectlCommand("get pods", "", "")

	if cmd.Env != nil {
		t.Fatalf("expected nil command environment, got %v", cmd.Env)
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

func hasEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
