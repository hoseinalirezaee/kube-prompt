package kube

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func parseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", raw, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy URL scheme %q: must be http, https, or socks5h", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL %q: host is required", raw)
	}
	return u, nil
}

func ValidateProxyURL(raw string) error {
	_, err := parseProxyURL(raw)
	return err
}

func ProxyDisplayString(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.User = nil
	return u.String()
}

func KubeconfigProxyDisplayString(kubeconfig string) (string, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	rawConfig, err := loadingRules.Load()
	if err != nil {
		return "", err
	}
	contextConfig := rawConfig.Contexts[rawConfig.CurrentContext]
	if contextConfig == nil {
		return "", nil
	}
	clusterConfig := rawConfig.Clusters[contextConfig.Cluster]
	if clusterConfig == nil || clusterConfig.ProxyURL == "" {
		return "", nil
	}
	return ProxyDisplayString(clusterConfig.ProxyURL), nil
}

func newRESTConfig(kubeconfig, proxyURL string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)

	config, err := loader.ClientConfig()
	if err != nil {
		return nil, err
	}

	u, err := parseProxyURL(proxyURL)
	if err != nil {
		return nil, err
	}
	if u != nil {
		config.Proxy = http.ProxyURL(u)
	}
	return config, nil
}
