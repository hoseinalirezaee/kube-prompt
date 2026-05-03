package kube

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func resetResourceCache() {
	lastFetchedAt = new(sync.Map)
	fetchInFlight = new(sync.Map)
	daemonSetList = new(sync.Map)
	deploymentList = new(sync.Map)
	endpointList = new(sync.Map)
	ingressList = new(sync.Map)
	limitRangeList = new(sync.Map)
	replicaSetList = new(sync.Map)
	statefulSetList = new(sync.Map)
}

func assertSuggestionTexts(t *testing.T, suggestions []prompt.Suggest, expected []string) {
	t.Helper()

	actual := make([]string, len(suggestions))
	for i := range suggestions {
		actual[i] = suggestions[i].Text
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected suggestions %v, got %v", expected, actual)
	}
}

func TestGetDaemonSetSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "node-agent", Namespace: namespace},
	})

	fetchDaemonSetList(ctx, client, namespace)
	suggestions := getDaemonSetSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"node-agent"})
}

func TestGetEndpointsSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: namespace},
	})

	fetchEndpoints(ctx, client, namespace)
	suggestions := getEndpointsSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"api"})
}

func TestGetDeploymentSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: namespace},
	})

	fetchDeployments(ctx, client, namespace)
	suggestions := getDeploymentSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"web"})
}

func TestGetIngressSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: namespace},
	})

	fetchIngresses(ctx, client, namespace)
	suggestions := getIngressSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"web"})
}

func TestGetLimitRangeSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: namespace},
	})

	fetchLimitRangeList(ctx, client, namespace)
	suggestions := getLimitRangeSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"defaults"})
}

func TestGetReplicaSetSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "web-7d8c9", Namespace: namespace},
	})

	fetchReplicaSetList(ctx, client, namespace)
	suggestions := getReplicaSetSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"web-7d8c9"})
}

func TestGetStatefulSetSuggestions(t *testing.T) {
	resetResourceCache()
	ctx := context.Background()
	namespace := "default"
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: namespace},
	})

	fetchStatefulSets(ctx, client, namespace)
	suggestions := getStatefulSetSuggestions(ctx, client, namespace)

	assertSuggestionTexts(t, suggestions, []string{"db"})
}
