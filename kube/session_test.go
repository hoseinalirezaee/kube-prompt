package kube

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitialNamespacePrefersDefaultNamespace(t *testing.T) {
	path := writeKubeconfig(t, "from-config")

	namespace, err := initialNamespaceFromKubeconfig(path, "from-flag")
	if err != nil {
		t.Fatalf("expected initial namespace, got error %v", err)
	}
	if namespace != "from-flag" {
		t.Fatalf("expected default namespace to win, got %q", namespace)
	}
}

func TestInitialNamespaceUsesExplicitKubeconfigNamespace(t *testing.T) {
	path := writeKubeconfig(t, "from-config")

	namespace, err := initialNamespaceFromKubeconfig(path, "")
	if err != nil {
		t.Fatalf("expected initial namespace, got error %v", err)
	}
	if namespace != "from-config" {
		t.Fatalf("expected kubeconfig namespace, got %q", namespace)
	}
}

func TestInitialNamespaceIgnoresImplicitKubeconfigDefault(t *testing.T) {
	path := writeKubeconfig(t, "")

	namespace, err := initialNamespaceFromKubeconfig(path, "")
	if err != nil {
		t.Fatalf("expected initial namespace, got error %v", err)
	}
	if namespace != "" {
		t.Fatalf("expected no namespace, got %q", namespace)
	}
}

func writeKubeconfig(t *testing.T, namespace string) string {
	t.Helper()

	namespaceLine := ""
	if namespace != "" {
		namespaceLine = "    namespace: " + namespace + "\n"
	}
	data := `apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test
  cluster:
    server: https://127.0.0.1
users:
- name: test
  user:
    token: test
contexts:
- name: test
  context:
    cluster: test
` + namespaceLine + `    user: test
`
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	return path
}
