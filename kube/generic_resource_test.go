package kube

import (
	"context"
	"testing"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestGenericResourceNameCompletionUsesDynamicClientForCRD(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	client := fake.NewSimpleClientset()
	client.Resources = discoveredResourceLists("example.com/v1", metav1.APIResource{
		Name:         "widgets",
		SingularName: "widget",
		ShortNames:   []string{"wdg"},
		Namespaced:   true,
		Verbs:        metav1.Verbs{"get", "list"},
	})
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "WidgetList"},
		testUnstructured("example.com/v1", "Widget", namespace, "alpha"),
		testUnstructured("example.com/v1", "Widget", namespace, "bravo"),
	)
	c := &Completer{client: client, dynamicClient: dynamicClient}

	fetchDiscoveredResources(ctx, client)
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.argumentsCompleter(ctx, namespace, []string{"get", "widgets", "alp"})
	}, []string{"alpha"})

	assertDynamicListAction(t, dynamicClient.Actions(), gvr, namespace)
}

func TestGenericResourceNameCompletionSupportsRoleBindings(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	gvr := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}
	client := fake.NewSimpleClientset()
	client.Resources = discoveredResourceLists("rbac.authorization.k8s.io/v1", metav1.APIResource{
		Name:         "rolebindings",
		SingularName: "rolebinding",
		Namespaced:   true,
		Verbs:        metav1.Verbs{"get", "list", "delete", "patch", "update"},
	})
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "RoleBindingList"},
		testUnstructured("rbac.authorization.k8s.io/v1", "RoleBinding", namespace, "read-secrets"),
	)
	c := &Completer{client: client, dynamicClient: dynamicClient}

	fetchDiscoveredResources(ctx, client)
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.argumentsCompleter(ctx, namespace, []string{"get", "rolebindings", "read"})
	}, []string{"read-secrets"})
	assertSuggestionTexts(t, c.argumentsCompleter(ctx, namespace, []string{"get", "rolebinding", "read"}), []string{"read-secrets"})
}

func TestGenericResourceNameCompletionSupportsClusterRoleBindings(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	gvr := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}
	client := fake.NewSimpleClientset()
	client.Resources = discoveredResourceLists("rbac.authorization.k8s.io/v1", metav1.APIResource{
		Name:         "clusterrolebindings",
		SingularName: "clusterrolebinding",
		Namespaced:   false,
		Verbs:        metav1.Verbs{"get", "list"},
	})
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "ClusterRoleBindingList"},
		testUnstructured("rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "", "cluster-admins"),
	)
	c := &Completer{client: client, dynamicClient: dynamicClient}

	fetchDiscoveredResources(ctx, client)
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.argumentsCompleter(ctx, "", []string{"get", "clusterrolebindings", "cluster"})
	}, []string{"cluster-admins"})

	assertDynamicListAction(t, dynamicClient.Actions(), gvr, "")
}

func TestGenericResourceNameCompletionRequiresListVerb(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	client := fake.NewSimpleClientset()
	client.Resources = discoveredResourceLists("example.com/v1", metav1.APIResource{
		Name:       "widgets",
		Namespaced: true,
		Verbs:      metav1.Verbs{"get"},
	})
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "WidgetList"},
		testUnstructured("example.com/v1", "Widget", namespace, "alpha"),
	)
	c := &Completer{client: client, dynamicClient: dynamicClient}

	fetchDiscoveredResources(ctx, client)
	assertSuggestionTexts(t, c.argumentsCompleter(ctx, namespace, []string{"get", "widgets", "a"}), []string{})
	assertNoDynamicActions(t, dynamicClient.Actions())
}

func TestGenericResourceNameCompletionSkipsNamespacedResourceWithoutNamespace(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	client := fake.NewSimpleClientset()
	client.Resources = discoveredResourceLists("example.com/v1", metav1.APIResource{
		Name:       "widgets",
		Namespaced: true,
		Verbs:      metav1.Verbs{"get", "list"},
	})
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "WidgetList"},
		testUnstructured("example.com/v1", "Widget", namespace, "alpha"),
	)
	c := &Completer{client: client, dynamicClient: dynamicClient}

	fetchDiscoveredResources(ctx, client)
	assertSuggestionTexts(t, c.argumentsCompleter(ctx, "", []string{"get", "widgets", "a"}), []string{})
	assertSuggestionTexts(t, c.argumentsCompleterWithScope(ctx, namespace, true, []string{"get", "widgets", "a"}), []string{})
	assertNoDynamicActions(t, dynamicClient.Actions())
}

func discoveredResourceLists(groupVersion string, resources ...metav1.APIResource) []*metav1.APIResourceList {
	return []*metav1.APIResourceList{
		{
			GroupVersion: groupVersion,
			APIResources: resources,
		},
	}
}

func testUnstructured(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	metadata := map[string]interface{}{"name": name}
	if namespace != "" {
		metadata["namespace"] = namespace
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   metadata,
		},
	}
}

func assertDynamicListAction(t *testing.T, actions []ktesting.Action, gvr schema.GroupVersionResource, namespace string) {
	t.Helper()

	var count int
	for _, action := range actions {
		if action.GetVerb() != "list" || action.GetResource() != gvr || action.GetNamespace() != namespace {
			continue
		}
		count++
	}
	if count != 1 {
		t.Fatalf("expected one dynamic list for %s in namespace %q, got actions %#v", gvr.String(), namespace, actions)
	}
}

func assertNoDynamicActions(t *testing.T, actions []ktesting.Action) {
	t.Helper()

	if len(actions) != 0 {
		t.Fatalf("expected no dynamic actions, got %#v", actions)
	}
}
