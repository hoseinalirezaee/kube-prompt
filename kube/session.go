package kube

import (
	"strings"
	"sync"

	"k8s.io/client-go/tools/clientcmd"
)

type SessionState struct {
	mu        sync.RWMutex
	namespace string
}

func NewSessionState(namespace string) *SessionState {
	s := &SessionState{}
	s.SetNamespace(namespace)
	return s
}

func (s *SessionState) Namespace() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.namespace
}

func (s *SessionState) SetNamespace(namespace string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.namespace = strings.TrimSpace(namespace)
}

func initialNamespaceFromKubeconfig(kubeconfig, defaultNamespace string) (string, error) {
	if ns := strings.TrimSpace(defaultNamespace); ns != "" {
		return ns, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	return explicitCurrentContextNamespace(loadingRules)
}

func explicitCurrentContextNamespace(loadingRules *clientcmd.ClientConfigLoadingRules) (string, error) {
	rawConfig, err := loadingRules.Load()
	if err != nil {
		return "", err
	}
	contextName := rawConfig.CurrentContext
	if contextName == "" {
		return "", nil
	}
	contextConfig, ok := rawConfig.Contexts[contextName]
	if !ok || contextConfig == nil {
		return "", nil
	}
	return strings.TrimSpace(contextConfig.Namespace), nil
}
