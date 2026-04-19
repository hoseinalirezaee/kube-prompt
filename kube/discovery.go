package kube

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/c-bata/go-prompt"
	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

const discoveryResourcesCacheKey = "discovery_resources"

var discoveredResourceList atomic.Value

type discoveredResource struct {
	Name         string
	SingularName string
	ShortNames   []string
	Namespaced   bool
	GroupVersion string
	Verbs        []string
}

func fetchDiscoveredResources(ctx context.Context, client kubernetes.Interface) {
	if !shouldFetch(discoveryResourcesCacheKey) {
		return
	}
	updateLastFetchedAt(discoveryResourcesCacheKey)

	select {
	case <-ctx.Done():
		discoveredResourceList.Store([]discoveredResource{})
		return
	default:
	}

	lists, err := discovery.ServerPreferredResources(client.Discovery())
	if err != nil {
		debug.Log(err.Error())
	}
	discoveredResourceList.Store(toDiscoveredResources(lists))
}

func getDiscoveredResources(ctx context.Context, client kubernetes.Interface) []discoveredResource {
	fetchDiscoveredResources(ctx, client)

	x := discoveredResourceList.Load()
	resources, ok := x.([]discoveredResource)
	if !ok {
		return []discoveredResource{}
	}
	return resources
}

func toDiscoveredResources(lists []*metav1.APIResourceList) []discoveredResource {
	resources := make([]discoveredResource, 0)
	for _, list := range lists {
		for i := range list.APIResources {
			resource := list.APIResources[i]
			if strings.Contains(resource.Name, "/") {
				continue
			}
			resources = append(resources, discoveredResource{
				Name:         resource.Name,
				SingularName: resource.SingularName,
				ShortNames:   resource.ShortNames,
				Namespaced:   resource.Namespaced,
				GroupVersion: list.GroupVersion,
				Verbs:        resource.Verbs,
			})
		}
	}

	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Name == resources[j].Name {
			return resources[i].GroupVersion < resources[j].GroupVersion
		}
		return resources[i].Name < resources[j].Name
	})
	return resources
}

func getDiscoveredResourceTypeSuggestions(ctx context.Context, client kubernetes.Interface, command string) []prompt.Suggest {
	resources := getDiscoveredResources(ctx, client)
	suggestions := make([]prompt.Suggest, 0, len(resources))
	seen := make(map[string]struct{})

	for i := range resources {
		if !supportsResourceCommand(resources[i], command) {
			continue
		}
		addDiscoveredResourceSuggestion(&suggestions, seen, prompt.Suggest{
			Text:        resources[i].Name,
			Description: discoveredResourceDescription(resources[i]),
		})
		if resources[i].SingularName != "" && resources[i].SingularName != resources[i].Name {
			addDiscoveredResourceSuggestion(&suggestions, seen, prompt.Suggest{
				Text:        resources[i].SingularName,
				Description: "singular, " + discoveredResourceDescription(resources[i]),
			})
		}
		for _, shortName := range resources[i].ShortNames {
			if shortName == "" || shortName == resources[i].Name {
				continue
			}
			addDiscoveredResourceSuggestion(&suggestions, seen, prompt.Suggest{
				Text:        shortName,
				Description: "short name for " + resources[i].Name + ", " + discoveredResourceDescription(resources[i]),
			})
		}
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})
	return suggestions
}

func addDiscoveredResourceSuggestion(suggestions *[]prompt.Suggest, seen map[string]struct{}, suggestion prompt.Suggest) {
	if _, ok := seen[suggestion.Text]; ok {
		return
	}
	seen[suggestion.Text] = struct{}{}
	*suggestions = append(*suggestions, suggestion)
}

func resolveDiscoveredResource(ctx context.Context, client kubernetes.Interface, command, text string) (discoveredResource, bool) {
	resources := getDiscoveredResources(ctx, client)
	for i := range resources {
		if supportsResourceCommand(resources[i], command) && resources[i].Name == text {
			return resources[i], true
		}
	}
	for i := range resources {
		if supportsResourceCommand(resources[i], command) && resources[i].SingularName == text {
			return resources[i], true
		}
	}
	for i := range resources {
		if !supportsResourceCommand(resources[i], command) {
			continue
		}
		for _, shortName := range resources[i].ShortNames {
			if shortName == text {
				return resources[i], true
			}
		}
	}
	return discoveredResource{}, false
}

func supportsResourceCommand(resource discoveredResource, command string) bool {
	switch command {
	case "get":
		return hasDiscoveredResourceVerb(resource, "get") || hasDiscoveredResourceVerb(resource, "list")
	case "describe":
		return hasDiscoveredResourceVerb(resource, "get")
	case "delete":
		return hasDiscoveredResourceVerb(resource, "delete")
	case "edit":
		return hasDiscoveredResourceVerb(resource, "update") || hasDiscoveredResourceVerb(resource, "patch")
	case "patch":
		return hasDiscoveredResourceVerb(resource, "patch") || hasDiscoveredResourceVerb(resource, "update")
	case "rollout":
		return resource.Name == "deployments" || resource.Name == "daemonsets" || resource.Name == "statefulsets"
	case "explain":
		return true
	default:
		return true
	}
}

func hasDiscoveredResourceVerb(resource discoveredResource, verb string) bool {
	for _, resourceVerb := range resource.Verbs {
		if resourceVerb == verb {
			return true
		}
	}
	return false
}

func discoveredResourceDescription(resource discoveredResource) string {
	scope := "cluster-scoped"
	if resource.Namespaced {
		scope = "namespaced"
	}
	return resource.GroupVersion + ", " + scope
}
