package kube

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
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

func TestRewritePodOwnerShortcutDeployment(t *testing.T) {
	setFakeKubernetesClientFactory(t, fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "apps"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
	}))

	got, err := rewritePodOwnerShortcut(context.Background(), "get pods deployment/web", "/tmp/kubeconfig", "", "apps")
	if err != nil {
		t.Fatalf("expected rewrite to succeed, got %v", err)
	}
	if want := "get pods -l app=web"; got != want {
		t.Fatalf("expected rewritten command %q, got %q", want, got)
	}
}

func TestRewritePodOwnerShortcutRejectsMissingNamespace(t *testing.T) {
	setFakeKubernetesClientFactory(t, fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "kwok-demo"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
	}))

	_, err := rewritePodOwnerShortcut(context.Background(), "get pods deployment/web", "/tmp/kubeconfig", "", "")
	if err == nil || !strings.Contains(err.Error(), "require a namespace") {
		t.Fatalf("expected missing namespace error, got %v", err)
	}
}

func TestRewritePodOwnerShortcutStatefulSetPreservesPipeAndNamespace(t *testing.T) {
	setFakeKubernetesClientFactory(t, fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "data"},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "db"},
			},
		},
	}))

	got, err := rewritePodOwnerShortcut(context.Background(), "get pods statefulset/db -n data | grep db", "/tmp/kubeconfig", "", "apps")
	if err != nil {
		t.Fatalf("expected rewrite to succeed, got %v", err)
	}
	if want := "get pods -l app=db -n data | grep db"; got != want {
		t.Fatalf("expected rewritten command %q, got %q", want, got)
	}
}

func TestRewritePodOwnerShortcutRejectsSelectorAndAllNamespaces(t *testing.T) {
	if _, err := rewritePodOwnerShortcut(context.Background(), "get pods deployment/web -l app=web", "", "", "apps"); err == nil || !strings.Contains(err.Error(), "--selector") {
		t.Fatalf("expected selector conflict error, got %v", err)
	}
	if _, err := rewritePodOwnerShortcut(context.Background(), "get pods deployment/web -A", "", "", "apps"); err == nil || !strings.Contains(err.Error(), "--all-namespaces") {
		t.Fatalf("expected all-namespaces error, got %v", err)
	}
	if _, err := rewritePodOwnerShortcut(context.Background(), "get pods deployment/web pod-a", "", "", "apps"); err == nil || !strings.Contains(err.Error(), "additional resource names") {
		t.Fatalf("expected additional resource names error, got %v", err)
	}
}

func TestRewritePodOwnerShortcutRejectsUnknownOwner(t *testing.T) {
	setFakeKubernetesClientFactory(t, fake.NewSimpleClientset())

	_, err := rewritePodOwnerShortcut(context.Background(), "get pods deployment/missing", "/tmp/kubeconfig", "", "apps")
	if err == nil || !strings.Contains(err.Error(), "cannot resolve deployment/missing") {
		t.Fatalf("expected missing owner error, got %v", err)
	}
}

func setFakeKubernetesClientFactory(t *testing.T, client kubernetes.Interface) {
	t.Helper()

	previous := kubernetesClientFactory
	kubernetesClientFactory = func(kubeconfig, proxyURL string) (kubernetes.Interface, error) {
		return client, nil
	}
	t.Cleanup(func() {
		kubernetesClientFactory = previous
	})
}

func hasEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
