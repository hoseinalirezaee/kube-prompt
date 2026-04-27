package kube

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
	"github.com/hoseinalirezaee/kube-prompt/prompt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const podWatchInitialSyncTimeout = 2 * time.Second

type podWatchCache struct {
	client      kubernetes.Interface
	ctx         context.Context
	cancel      context.CancelFunc
	syncTimeout time.Duration

	mu       sync.Mutex
	watchers map[string]*podNamespaceWatcher
}

type podNamespaceWatcher struct {
	factory  informers.SharedInformerFactory
	informer cache.SharedIndexInformer
	lister   corev1listers.PodLister
}

func newPodWatchCache(ctx context.Context, client kubernetes.Interface) *podWatchCache {
	if ctx == nil {
		ctx = context.Background()
	}
	cacheCtx, cancel := context.WithCancel(ctx)
	return &podWatchCache{
		client:      client,
		ctx:         cacheCtx,
		cancel:      cancel,
		syncTimeout: podWatchInitialSyncTimeout,
		watchers:    make(map[string]*podNamespaceWatcher),
	}
}

func (p *podWatchCache) close() {
	if p == nil {
		return
	}
	p.cancel()
}

func (p *podWatchCache) watcher(namespace string) *podNamespaceWatcher {
	if p == nil || p.client == nil {
		return nil
	}

	p.mu.Lock()
	watcher, ok := p.watchers[namespace]
	if !ok {
		factory := informers.NewSharedInformerFactoryWithOptions(
			p.client,
			0,
			informers.WithNamespace(namespace),
		)
		pods := factory.Core().V1().Pods()
		watcher = &podNamespaceWatcher{
			factory:  factory,
			informer: pods.Informer(),
			lister:   pods.Lister(),
		}
		p.watchers[namespace] = watcher
		factory.StartWithContext(p.ctx)
	}
	p.mu.Unlock()

	if !ok {
		p.waitForInitialSync(namespace, watcher)
	}
	return watcher
}

func (p *podWatchCache) waitForInitialSync(namespace string, watcher *podNamespaceWatcher) {
	ctx, cancel := context.WithTimeout(p.ctx, p.syncTimeout)
	defer cancel()

	if !cache.WaitForCacheSync(ctx.Done(), watcher.informer.HasSynced) {
		debug.Log("timed out waiting for pod watch cache sync for namespace " + namespace)
	}
}

func (p *podWatchCache) pods(namespace string) []*corev1.Pod {
	watcher := p.watcher(namespace)
	if watcher == nil {
		return nil
	}

	var (
		pods []*corev1.Pod
		err  error
	)
	if namespace == "" {
		pods, err = watcher.lister.List(labels.Everything())
	} else {
		pods, err = watcher.lister.Pods(namespace).List(labels.Everything())
	}
	if err != nil {
		debug.Log(err.Error())
		return nil
	}

	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Name == pods[j].Name {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
	return pods
}

func (p *podWatchCache) pod(namespace, podName string) (corev1.Pod, bool) {
	for _, pod := range p.pods(namespace) {
		if pod.Name == podName {
			return *pod, true
		}
	}
	return corev1.Pod{}, false
}

func (c *Completer) ensurePodCache(ctx context.Context) *podWatchCache {
	if c == nil || c.client == nil {
		return nil
	}

	c.podCacheMu.Lock()
	defer c.podCacheMu.Unlock()

	if c.podCache == nil {
		c.podCache = newPodWatchCache(ctx, c.client)
	}
	return c.podCache
}

func (c *Completer) getPodSuggestions(ctx context.Context, namespace string) []prompt.Suggest {
	cache := c.ensurePodCache(ctx)
	if cache == nil {
		return []prompt.Suggest{}
	}

	pods := cache.pods(namespace)
	if len(pods) == 0 {
		return []prompt.Suggest{}
	}

	suggestions := make([]prompt.Suggest, len(pods))
	for i := range pods {
		suggestions[i] = prompt.Suggest{
			Text:        pods[i].Name,
			Description: string(pods[i].Status.Phase),
		}
	}
	return suggestions
}

func (c *Completer) getPod(ctx context.Context, namespace, podName string) (corev1.Pod, bool) {
	cache := c.ensurePodCache(ctx)
	if cache == nil {
		return corev1.Pod{}, false
	}
	return cache.pod(namespace, podName)
}

func (c *Completer) getPortsFromPodName(ctx context.Context, namespace string, podName string) []prompt.Suggest {
	pod, found := c.getPod(ctx, namespace, podName)
	if !found {
		return []prompt.Suggest{}
	}

	portSet := make(map[int32]struct{})
	for i := range pod.Spec.Containers {
		for j := range pod.Spec.Containers[i].Ports {
			portSet[pod.Spec.Containers[i].Ports[j].ContainerPort] = struct{}{}
		}
	}

	var ports []int
	for port := range portSet {
		ports = append(ports, int(port))
	}
	sort.Ints(ports)

	suggestions := make([]prompt.Suggest, 0, len(ports))
	for i := range ports {
		suggestions = append(suggestions, prompt.Suggest{
			Text: fmt.Sprintf("%d:%d", ports[i], ports[i]),
		})
	}
	return suggestions
}

func (c *Completer) getContainerNamesFromCachedPods(ctx context.Context, namespace string) []prompt.Suggest {
	cache := c.ensurePodCache(ctx)
	if cache == nil {
		return []prompt.Suggest{}
	}

	pods := cache.pods(namespace)
	if len(pods) == 0 {
		return []prompt.Suggest{}
	}

	containerToPod := make(map[string]string, len(pods))
	for i := range pods {
		for j := range pods[i].Spec.Containers {
			containerToPod[pods[i].Spec.Containers[j].Name] = pods[i].Name
		}
	}

	suggestions := make([]prompt.Suggest, 0, len(containerToPod))
	for container, pod := range containerToPod {
		suggestions = append(suggestions, prompt.Suggest{
			Text:        container,
			Description: "Pod Name: " + pod,
		})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Text == suggestions[j].Text {
			return suggestions[i].Description < suggestions[j].Description
		}
		return suggestions[i].Text < suggestions[j].Text
	})
	return suggestions
}

func (c *Completer) getContainerName(ctx context.Context, namespace string, podName string) []prompt.Suggest {
	pod, found := c.getPod(ctx, namespace, podName)
	if !found {
		return []prompt.Suggest{}
	}

	suggestions := make([]prompt.Suggest, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		suggestions[i] = prompt.Suggest{
			Text: pod.Spec.Containers[i].Name,
		}
	}
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})
	return suggestions
}
