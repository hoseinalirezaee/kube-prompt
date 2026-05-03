package kube

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestPodWatchCacheUpdatesAfterPodReplacement(t *testing.T) {
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(testPod(namespace, "web-old", nil))
	c := &Completer{client: client}
	defer c.Close()

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPodSuggestions(ctx, namespace)
	}, []string{"web-old"})

	if err := client.CoreV1().Pods(namespace).Delete(ctx, "web-old", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete old pod: %v", err)
	}
	if _, err := client.CoreV1().Pods(namespace).Create(ctx, testPod(namespace, "web-new", nil), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create new pod: %v", err)
	}

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPodSuggestions(ctx, namespace)
	}, []string{"web-new"})
}

func TestPodWatchCacheDoesNotBlockOnInitialSync(t *testing.T) {
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(testPod(namespace, "web", nil))
	client.Fake.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		time.Sleep(200 * time.Millisecond)
		return false, nil, nil
	})
	c := &Completer{client: client}
	defer c.Close()

	start := time.Now()
	suggestions := c.getPodSuggestions(ctx, namespace)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("expected pod suggestions to return promptly, took %s", elapsed)
	}
	assertSuggestionTexts(t, suggestions, []string{})

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPodSuggestions(ctx, namespace)
	}, []string{"web"})
}

func TestPodWatchCacheKeepsNamespacesIsolated(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		testPod("default", "web-default", nil),
		testPod("apps", "web-apps", nil),
	)
	c := &Completer{client: client}
	defer c.Close()

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPodSuggestions(ctx, "default")
	}, []string{"web-default"})
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPodSuggestions(ctx, "apps")
	}, []string{"web-apps"})

	if _, err := client.CoreV1().Pods("apps").Create(ctx, testPod("apps", "worker-apps", nil), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create apps pod: %v", err)
	}

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPodSuggestions(ctx, "apps")
	}, []string{"web-apps", "worker-apps"})
	assertSuggestionTexts(t, c.getPodSuggestions(ctx, "default"), []string{"web-default"})
}

func TestPodWatchCacheUpdatesContainerAndPortSuggestions(t *testing.T) {
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(testPod(namespace, "web", []corev1.Container{
		{Name: "old", Ports: []corev1.ContainerPort{{ContainerPort: 8080}}},
	}))
	c := &Completer{client: client}
	defer c.Close()

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getContainerName(ctx, namespace, "web")
	}, []string{"old"})
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPortsFromPodName(ctx, namespace, "web")
	}, []string{"8080:8080"})

	if err := client.CoreV1().Pods(namespace).Delete(ctx, "web", metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete old pod: %v", err)
	}
	if _, err := client.CoreV1().Pods(namespace).Create(ctx, testPod(namespace, "web", []corev1.Container{
		{Name: "api", Ports: []corev1.ContainerPort{{ContainerPort: 9090}}},
	}), metav1.CreateOptions{}); err != nil {
		t.Fatalf("create new pod: %v", err)
	}

	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getContainerName(ctx, namespace, "web")
	}, []string{"api"})
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getContainerNamesFromCachedPods(ctx, namespace)
	}, []string{"api"})
	waitForSuggestionTexts(t, func() []prompt.Suggest {
		return c.getPortsFromPodName(ctx, namespace, "web")
	}, []string{"9090:9090"})
}

func testPod(namespace, name string, containers []corev1.Container) *corev1.Pod {
	if containers == nil {
		containers = []corev1.Container{{Name: "app"}}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func waitForSuggestionTexts(t *testing.T, suggestions func() []prompt.Suggest, expected []string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		actual := suggestionTexts(suggestions())
		if reflect.DeepEqual(actual, expected) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected suggestions %v, got %v", expected, actual)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func suggestionTexts(suggestions []prompt.Suggest) []string {
	texts := make([]string, len(suggestions))
	for i := range suggestions {
		texts[i] = suggestions[i].Text
	}
	return texts
}
