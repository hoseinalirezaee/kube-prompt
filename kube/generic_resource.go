package kube

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
	"github.com/hoseinalirezaee/kube-prompt/prompt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	genericResourceNameListLimit   int64         = 500
	genericResourceNameListTimeout time.Duration = 2 * time.Second
)

var genericResourceNameList *sync.Map

func (c *Completer) getGenericResourceNameSuggestions(ctx context.Context, namespace string, allNamespaces bool, resource discoveredResource) []prompt.Suggest {
	if c == nil || c.dynamicClient == nil {
		return []prompt.Suggest{}
	}
	namespace = strings.TrimSpace(namespace)
	if !hasDiscoveredResourceVerb(resource, "list") {
		return []prompt.Suggest{}
	}
	if resource.Namespaced && (namespace == "" || allNamespaces) {
		return []prompt.Suggest{}
	}
	if !resource.Namespaced {
		namespace = ""
	}

	gvr, ok := discoveredResourceGVR(resource)
	if !ok {
		return []prompt.Suggest{}
	}
	key := genericResourceNameCacheKey(resource, namespace)
	if shouldFetch(key) {
		go fetchGenericResourceNames(ctx, c.dynamicClient, key, gvr, resource.Namespaced, namespace)
	}

	x, ok := genericResourceNameList.Load(key)
	if !ok {
		return []prompt.Suggest{}
	}
	suggestions, ok := x.([]prompt.Suggest)
	if !ok {
		return []prompt.Suggest{}
	}
	return suggestions
}

func fetchGenericResourceNames(ctx context.Context, client dynamic.Interface, key string, gvr schema.GroupVersionResource, namespaced bool, namespace string) {
	fetch, ok := beginFetch(key)
	if !ok {
		return
	}
	defer fetch.Done()

	fetchCtx, cancel := context.WithTimeout(ctx, genericResourceNameListTimeout)
	defer cancel()

	resourceClient := client.Resource(gvr)
	var (
		list *unstructured.UnstructuredList
		err  error
	)
	if namespaced {
		list, err = resourceClient.Namespace(namespace).List(fetchCtx, metav1.ListOptions{Limit: genericResourceNameListLimit})
	} else {
		list, err = resourceClient.List(fetchCtx, metav1.ListOptions{Limit: genericResourceNameListLimit})
	}
	if err != nil {
		debug.Log(err.Error())
		genericResourceNameList.Store(key, []prompt.Suggest{})
		return
	}
	genericResourceNameList.Store(key, genericResourceNameSuggestions(list))
}

func genericResourceNameSuggestions(list *unstructured.UnstructuredList) []prompt.Suggest {
	if list == nil || len(list.Items) == 0 {
		return []prompt.Suggest{}
	}

	suggestions := make([]prompt.Suggest, 0, len(list.Items))
	seen := make(map[string]struct{}, len(list.Items))
	for i := range list.Items {
		name := list.Items[i].GetName()
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		suggestions = append(suggestions, prompt.Suggest{Text: name})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})
	return suggestions
}

func discoveredResourceGVR(resource discoveredResource) (schema.GroupVersionResource, bool) {
	gv, err := schema.ParseGroupVersion(resource.GroupVersion)
	if err != nil || gv.Version == "" || resource.Name == "" {
		return schema.GroupVersionResource{}, false
	}
	return gv.WithResource(resource.Name), true
}

func genericResourceNameCacheKey(resource discoveredResource, namespace string) string {
	return "generic_resource_name:" + resource.GroupVersion + ":" + resource.Name + ":" + strings.TrimSpace(namespace)
}
