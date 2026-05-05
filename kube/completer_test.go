package kube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	"github.com/hoseinalirezaee/kube-prompt/prompt/completer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewCompleterUsesExplicitProxy(t *testing.T) {
	proxyRequests := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests <- r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[{"metadata":{"name":"default"}}]}`))
	}))
	defer proxy.Close()

	kubeconfig := writeKubeconfigForServer(t, "http://kube-api.invalid", "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := NewCompleter(ctx, kubeconfig, NewSessionState(""), "", proxy.URL)
	if err != nil {
		t.Fatalf("expected completer through proxy, got error %v", err)
	}
	defer c.Close()

	select {
	case got := <-proxyRequests:
		if !strings.HasPrefix(got, "http://kube-api.invalid/api/v1/namespaces") {
			t.Fatalf("expected namespace request through proxy, got %q", got)
		}
	default:
		t.Fatal("expected proxy to receive namespace request")
	}
}

func TestNewCompleterExplicitProxyOverridesKubeconfigProxy(t *testing.T) {
	proxyRequests := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests <- r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`))
	}))
	defer proxy.Close()

	kubeconfig := writeKubeconfigForServer(t, "http://kube-api.invalid", "http://127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	c, err := NewCompleter(ctx, kubeconfig, NewSessionState(""), "", proxy.URL)
	if err != nil {
		t.Fatalf("expected completer through explicit proxy, got error %v", err)
	}
	defer c.Close()

	select {
	case <-proxyRequests:
	default:
		t.Fatal("expected explicit proxy to receive namespace request")
	}
}

func TestNewRESTConfigAcceptsSocks5hProxy(t *testing.T) {
	kubeconfig := writeKubeconfigForServer(t, "http://kube-api.invalid", "")

	config, err := newRESTConfig(kubeconfig, "socks5h://user:pass@proxy.example:1080")
	if err != nil {
		t.Fatalf("expected socks5h proxy config, got error %v", err)
	}
	if config.Proxy == nil {
		t.Fatal("expected proxy function to be set")
	}

	got, err := config.Proxy(&http.Request{URL: &url.URL{Scheme: "http", Host: "kube-api.invalid"}})
	if err != nil {
		t.Fatalf("expected proxy function to succeed, got %v", err)
	}
	if got.String() != "socks5h://user:pass@proxy.example:1080" {
		t.Fatalf("expected socks5h proxy URL, got %q", got.String())
	}
}

func writeKubeconfigForServer(t *testing.T, server, proxyURL string) string {
	t.Helper()

	proxyLine := ""
	if proxyURL != "" {
		proxyLine = "    proxy-url: " + proxyURL + "\n"
	}
	data := `apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test
  cluster:
    server: ` + server + `
` + proxyLine + `users:
- name: test
  user:
    token: test
contexts:
- name: test
  context:
    cluster: test
    user: test
`
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("failed to write kubeconfig: %v", err)
	}
	return path
}

func TestSessionCommandCompletion(t *testing.T) {
	b := prompt.NewBuffer()
	b.InsertText("/", false, true)
	c := &Completer{}

	assertSuggestionContains(t, c.Complete(*b.Document()), "help", "shortcuts")
	assertSuggestionContains(t, c.Complete(*b.Document()), "namespace", "active namespace")
	assertSuggestionContains(t, c.Complete(*b.Document()), "output", "Save captured")
	assertSuggestionContains(t, c.Complete(*b.Document()), "outputs", "Browse captured")
	assertSuggestionContains(t, c.Complete(*b.Document()), "exit", "Exit")
}

func TestSessionCommandCompletionAcceptedAfterSlash(t *testing.T) {
	tests := []struct {
		input      string
		suggestion string
		want       string
	}{
		{input: "/", suggestion: "namespace", want: "/namespace"},
		{input: "/n", suggestion: "namespace", want: "/namespace"},
		{input: "/h", suggestion: "help", want: "/help"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			b := prompt.NewBuffer()
			b.InsertText(tt.input, false, true)
			c := &Completer{}
			suggestions := c.Complete(*b.Document())
			assertSuggestionContains(t, suggestions, tt.suggestion, "")

			if got := acceptSuggestionForTest(b, tt.suggestion); got != tt.want {
				t.Fatalf("expected accepted suggestion to produce %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNamespaceSessionCommandSuggestsClusterNamespaces(t *testing.T) {
	b := prompt.NewBuffer()
	b.InsertText("/namespace pr", false, true)
	c := &Completer{
		namespaceList: &corev1.NamespaceList{
			Items: []corev1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "staging"}},
			},
		},
	}

	assertSuggestionTexts(t, c.Complete(*b.Document()), []string{"prod"})
}

func TestCompleterDelegatesCurrentPipeSegmentToShellCompleter(t *testing.T) {
	b := prompt.NewBuffer()
	b.InsertText("get pods | gr", false, true)
	c := &Completer{
		shellCompleter: fakeShellCompleter{
			" gr": {{Text: "grep", Description: "command"}},
		},
	}

	assertSuggestionTexts(t, c.Complete(*b.Document()), []string{"grep"})
}

func TestCompleterUsesLastPipeSegment(t *testing.T) {
	b := prompt.NewBuffer()
	b.InsertText("get pods | grep web | aw", false, true)
	c := &Completer{
		shellCompleter: fakeShellCompleter{
			" aw": {{Text: "awk", Description: "command"}},
		},
	}

	assertSuggestionTexts(t, c.Complete(*b.Document()), []string{"awk"})
}

func TestCompleterSuggestsSecretDecodePipeCommand(t *testing.T) {
	b := prompt.NewBuffer()
	b.InsertText("get secret api-credentials | kpb", false, true)
	c := &Completer{}

	assertSuggestionContains(t, c.Complete(*b.Document()), SecretDecodeCommand, "Secret data")
}

func TestCompleterPreservesKubectlCompletionBeforePipe(t *testing.T) {
	resetDiscoveryCache()
	resetResourceCache()
	b := prompt.NewBuffer()
	b.InsertText("get po", false, true)
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
	c := &Completer{client: client}

	fetchDiscoveredResources(context.Background(), client)
	assertSuggestionContains(t, c.Complete(*b.Document()), "pods", "v1")
}

func TestCheckNamespaceArgSupportsKubectlForms(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "get pods --namespace apps", want: "apps"},
		{input: "get pods --namespace=apps", want: "apps"},
		{input: "get pods -n apps", want: "apps"},
		{input: "get pods -n=apps", want: "apps"},
		{input: "get pods -napps", want: "apps"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			b := prompt.NewBuffer()
			b.InsertText(tt.input, false, true)
			if got := checkNamespaceArg(*b.Document()); got != tt.want {
				t.Fatalf("expected namespace %q, got %q", tt.want, got)
			}
		})
	}
}

func acceptSuggestionForTest(b *prompt.Buffer, text string) string {
	w := b.Document().GetWordBeforeCursorUntilSeparator(completer.FilePathCompletionSeparator)
	if w != "" {
		b.DeleteBeforeCursor(len([]rune(w)))
	}
	b.InsertText(text, false, true)
	return b.Text()
}

type fakeShellCompleter map[string][]prompt.Suggest

func (f fakeShellCompleter) Complete(segment string) []prompt.Suggest {
	return f[segment]
}
