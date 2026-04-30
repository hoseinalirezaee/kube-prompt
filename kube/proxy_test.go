package kube

import "testing"

func TestValidateProxyURLSupportsExpectedSchemes(t *testing.T) {
	for _, proxyURL := range []string{
		"http://proxy.example:8080",
		"https://proxy.example:8443",
		"socks5h://proxy.example:1080",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			if err := ValidateProxyURL(proxyURL); err != nil {
				t.Fatalf("expected proxy URL to be valid, got %v", err)
			}
		})
	}
}

func TestValidateProxyURLRejectsUnsupportedSchemes(t *testing.T) {
	for _, proxyURL := range []string{
		"socks5://proxy.example:1080",
		"ftp://proxy.example",
		"http:///missing-host",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			if err := ValidateProxyURL(proxyURL); err == nil {
				t.Fatal("expected proxy URL validation error")
			}
		})
	}
}

func TestProxyDisplayStringStripsCredentials(t *testing.T) {
	got := ProxyDisplayString("https://user:pass@proxy.example:8443/path?q=1")
	want := "https://proxy.example:8443/path?q=1"
	if got != want {
		t.Fatalf("expected proxy display %q, got %q", want, got)
	}
}

func TestKubeconfigProxyDisplayString(t *testing.T) {
	kubeconfig := writeKubeconfigForServer(t, "http://kube-api.invalid", "https://user:pass@proxy.example:8443")

	got, err := KubeconfigProxyDisplayString(kubeconfig)
	if err != nil {
		t.Fatalf("expected kubeconfig proxy display, got error %v", err)
	}
	want := "https://proxy.example:8443"
	if got != want {
		t.Fatalf("expected proxy display %q, got %q", want, got)
	}
}
