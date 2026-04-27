package kube

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func resetDiscoveryCache() {
	lastFetchedAt = new(sync.Map)
	discoveredResourceList = atomic.Value{}
}

func TestDiscoveredResourceTypeSuggestionsIncludeCRD(t *testing.T) {
	resetDiscoveryCache()
	client := fake.NewSimpleClientset()
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "example.com/v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "widgets",
					SingularName: "widget",
					ShortNames:   []string{"wdg"},
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list", "delete", "update"},
				},
			},
		},
	}

	suggestions := getDiscoveredResourceTypeSuggestions(context.Background(), client, "get")

	assertSuggestionContains(t, suggestions, "widgets", "example.com/v1", "namespaced")
	assertSuggestionContains(t, suggestions, "widget", "singular", "example.com/v1")
	assertSuggestionContains(t, suggestions, "wdg", "short name", "widgets")
}

func TestDiscoveredResourceTypeSuggestionsDoNotUseRemovedStaticResources(t *testing.T) {
	resetDiscoveryCache()
	client := fake.NewSimpleClientset()
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "pods",
					SingularName: "pod",
					ShortNames:   []string{"po"},
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list"},
				},
			},
		},
	}

	suggestions := getDiscoveredResourceTypeSuggestions(context.Background(), client, "get")

	if hasSuggestionText(suggestions, "thirdpartyresources") {
		t.Fatal("thirdpartyresources should not be suggested when discovery does not return it")
	}
	if hasSuggestionText(suggestions, "podsecuritypolicies") {
		t.Fatal("podsecuritypolicies should not be suggested when discovery does not return it")
	}
	assertSuggestionContains(t, suggestions, "pods", "v1", "namespaced")
}

func TestDiscoveredResourceTypeSuggestionsFilterByCommandVerb(t *testing.T) {
	resetDiscoveryCache()
	client := fake.NewSimpleClientset()
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "deployments",
					SingularName: "deployment",
					ShortNames:   []string{"deploy"},
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list", "delete", "update"},
				},
				{
					Name:         "controllers",
					SingularName: "controller",
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list"},
				},
			},
		},
	}

	suggestions := getDiscoveredResourceTypeSuggestions(context.Background(), client, "delete")

	assertSuggestionContains(t, suggestions, "deployments", "apps/v1", "namespaced")
	if hasSuggestionText(suggestions, "controllers") {
		t.Fatal("resources without delete should not be suggested for delete")
	}
}

func TestDiscoveredResourceNameCompletionUsesDiscoveredShortName(t *testing.T) {
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
					Verbs:        metav1.Verbs{"get", "list", "delete"},
				},
			},
		},
	}
	c := &Completer{client: client}
	defer c.Close()

	suggestions := c.argumentsCompleter(ctx, namespace, []string{"get", "po", "web"})

	assertSuggestionTexts(t, suggestions, []string{"web-0"})
}

func TestDiscoveredCRDNameCompletionIsGracefullyEmpty(t *testing.T) {
	resetDiscoveryCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset()
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "example.com/v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "widgets",
					SingularName: "widget",
					ShortNames:   []string{"wdg"},
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list"},
				},
			},
		},
	}
	c := &Completer{client: client}

	suggestions := c.argumentsCompleter(ctx, namespace, []string{"get", "widgets", ""})

	assertSuggestionTexts(t, suggestions, []string{})
}

func TestDiscoveredBuiltInResourceNameCompletionRequiresDiscovery(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: namespace},
	})
	client.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "deployments",
					SingularName: "deployment",
					ShortNames:   []string{"deploy"},
					Namespaced:   true,
					Verbs:        metav1.Verbs{"get", "list"},
				},
			},
		},
	}
	c := &Completer{client: client}

	fetchDeployments(ctx, client, namespace)
	suggestions := c.argumentsCompleter(ctx, namespace, []string{"get", "deploy", "we"})

	assertSuggestionTexts(t, suggestions, []string{"web"})
}

func assertSuggestionContains(t *testing.T, suggestions []prompt.Suggest, text string, descriptionParts ...string) {
	t.Helper()

	for _, suggestion := range suggestions {
		if suggestion.Text != text {
			continue
		}
		for _, part := range descriptionParts {
			if !strings.Contains(suggestion.Description, part) {
				t.Fatalf("suggestion %q description %q does not contain %q", text, suggestion.Description, part)
			}
		}
		return
	}
	t.Fatalf("expected suggestion %q in %#v", text, suggestions)
}

func hasSuggestionText(suggestions []prompt.Suggest, text string) bool {
	for _, suggestion := range suggestions {
		if suggestion.Text == text {
			return true
		}
	}
	return false
}
