package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCommandsIncludeKubectlTopLevelCommands(t *testing.T) {
	expected := []string{
		"annotate",
		"apply",
		"attach",
		"auth",
		"autoscale",
		"certificate",
		"cluster-info",
		"completion",
		"config",
		"cordon",
		"cp",
		"create",
		"debug",
		"delete",
		"describe",
		"diff",
		"drain",
		"edit",
		"events",
		"exec",
		"explain",
		"expose",
		"get",
		"kustomize",
		"label",
		"logs",
		"patch",
		"plugin",
		"port-forward",
		"proxy",
		"replace",
		"rollout",
		"run",
		"scale",
		"set",
		"taint",
		"top",
		"uncordon",
		"wait",
		"api-resources",
		"api-versions",
		"version",
	}

	for _, command := range expected {
		if !hasSuggestionText(commands, command) {
			t.Fatalf("expected top-level command %q", command)
		}
	}
}

func TestGroupedCommandSubcommands(t *testing.T) {
	c := &Completer{client: fake.NewSimpleClientset()}
	ctx := context.Background()

	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"create", "cluster"}), "clusterrole", "cluster role")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"create", "secret", "g"}), "generic", "local file")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"create", "service", "node"}), "nodeport", "NodePort")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"auth", "can"}), "can-i", "allowed")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"certificate", "a"}), "approve", "certificate")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"completion", "p"}), "powershell")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"plugin", "l"}), "list", "plugin")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"rollout", "res"}), "restart", "Restart")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, "default", []string{"set", "im"}), "image", "pod template")
}

func TestGenericResourceCommandsUseDiscovery(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: namespace},
	})
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "pods",
					SingularName: "pod",
					ShortNames:   []string{"po"},
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list", "patch", "update", "delete"},
				},
			},
		},
	}
	c := &Completer{client: client}
	defer c.Close()

	assertSuggestionContains(t, c.argumentsCompleter(ctx, namespace, []string{"patch", "p"}), "pods", "v1")
	assertSuggestionTexts(t, c.argumentsCompleter(ctx, namespace, []string{"patch", "po", "web"}), []string{"web-0"})
	assertSuggestionContains(t, c.argumentsCompleter(ctx, namespace, []string{"label", "p"}), "pods", "v1")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, namespace, []string{"wait", "p"}), "pods", "v1")
	assertSuggestionContains(t, c.argumentsCompleter(ctx, namespace, []string{"set", "image", "p"}), "pods", "v1")
}

func TestNodeAndPodSpecificCommands(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: namespace}},
	)
	c := &Completer{client: client}
	defer c.Close()
	fetchNodeList(ctx, client)

	assertSuggestionTexts(t, c.argumentsCompleter(ctx, namespace, []string{"taint", "nodes", "work"}), []string{"worker-1"})
	assertSuggestionTexts(t, c.argumentsCompleter(ctx, namespace, []string{"debug", "web"}), []string{"web-0"})
	assertSuggestionTexts(t, c.argumentsCompleter(ctx, namespace, []string{"cp", "web"}), []string{"web-0"})
}
