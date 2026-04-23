package kube

import (
	"testing"

	"github.com/c-bata/go-prompt"
	"github.com/c-bata/go-prompt/completer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSessionCommandCompletion(t *testing.T) {
	b := prompt.NewBuffer()
	b.InsertText("/", false, true)
	c := &Completer{}

	assertSuggestionContains(t, c.Complete(*b.Document()), "namespace", "active namespace")
	assertSuggestionContains(t, c.Complete(*b.Document()), "exit", "Exit")
}

func TestSessionCommandCompletionAcceptedAfterSlash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "/", want: "/namespace"},
		{input: "/n", want: "/namespace"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			b := prompt.NewBuffer()
			b.InsertText(tt.input, false, true)
			c := &Completer{}
			suggestions := c.Complete(*b.Document())
			assertSuggestionContains(t, suggestions, "namespace", "active namespace")

			if got := acceptSuggestionForTest(b, "namespace"); got != tt.want {
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
