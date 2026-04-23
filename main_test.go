package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hoseinalirezaee/kube-prompt/kube"
	"github.com/hoseinalirezaee/kube-prompt/prompt"
)

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run([]string{arg}, &stdout, &stderr)

			if code != 0 {
				t.Fatalf("expected exit code 0, got %d", code)
			}
			if stderr.Len() != 0 {
				t.Fatalf("expected no stderr, got %q", stderr.String())
			}
			out := stdout.String()
			for _, expected := range []string{
				"Usage: kube-prompt [flags]",
				"without the kubectl prefix",
				"-h, --help",
				"-v, --version",
				"--kubeconfig PATH",
				"--default-namespace NAME",
				"get pods",
				"get pods | grep web",
			} {
				if !strings.Contains(out, expected) {
					t.Fatalf("expected help output to contain %q, got %q", expected, out)
				}
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	origVersion, origRevision := version, revision
	version, revision = "v1.2.3", "abc123"
	defer func() {
		version, revision = origVersion, origRevision
	}()

	for _, arg := range []string{"--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run([]string{arg}, &stdout, &stderr)

			if code != 0 {
				t.Fatalf("expected exit code 0, got %d", code)
			}
			if stderr.Len() != 0 {
				t.Fatalf("expected no stderr, got %q", stderr.String())
			}
			if got, want := strings.TrimSpace(stdout.String()), "kube-prompt v1.2.3 (rev-abc123)"; got != want {
				t.Fatalf("expected version %q, got %q", want, got)
			}
		})
	}
}

func TestRunRejectsInvalidCLIInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"--bad"}, want: "flag provided but not defined"},
		{name: "positional arg", args: []string{"get"}, want: "unexpected argument: get"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tt.args, &stdout, &stderr)

			if code != 2 {
				t.Fatalf("expected exit code 2, got %d", code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected no stdout, got %q", stdout.String())
			}
			errOut := stderr.String()
			if !strings.Contains(errOut, tt.want) {
				t.Fatalf("expected stderr to contain %q, got %q", tt.want, errOut)
			}
			if !strings.Contains(errOut, "Usage: kube-prompt [flags]") {
				t.Fatalf("expected stderr usage, got %q", errOut)
			}
		})
	}
}

func TestParseCLIKubeconfig(t *testing.T) {
	var stdout, stderr bytes.Buffer

	cfg, ok := parseCLI([]string{"--kubeconfig", "/tmp/kubeconfig", "--default-namespace", "apps"}, &stdout, &stderr)

	if !ok {
		t.Fatalf("expected parse to succeed, stderr %q", stderr.String())
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.kubeconfig != "/tmp/kubeconfig" {
		t.Fatalf("expected kubeconfig path, got %q", cfg.kubeconfig)
	}
	if cfg.defaultNamespace != "apps" {
		t.Fatalf("expected default namespace, got %q", cfg.defaultNamespace)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestRunRequiresKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "")
	var stdout, stderr bytes.Buffer

	code := run([]string{}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), errKubeconfigRequired.Error()) {
		t.Fatalf("expected kubeconfig error, got %q", stderr.String())
	}
}

func TestRequireKubeconfigUsesEnv(t *testing.T) {
	cfg, err := requireKubeconfig(cliConfig{}, func(key string) string {
		if key == "KUBECONFIG" {
			return "/tmp/env-kubeconfig"
		}
		return ""
	})

	if err != nil {
		t.Fatalf("expected kubeconfig from env, got error %v", err)
	}
	if cfg.kubeconfig != "" {
		t.Fatalf("expected explicit kubeconfig to stay empty, got %q", cfg.kubeconfig)
	}
	if cfg.kubeconfigStatus != "/tmp/env-kubeconfig" {
		t.Fatalf("expected status kubeconfig from env, got %q", cfg.kubeconfigStatus)
	}
}

func TestRequireKubeconfigPrefersFlag(t *testing.T) {
	cfg, err := requireKubeconfig(cliConfig{kubeconfig: "/tmp/flag-kubeconfig"}, func(string) string {
		return "/tmp/env-kubeconfig"
	})

	if err != nil {
		t.Fatalf("expected kubeconfig from flag, got error %v", err)
	}
	if cfg.kubeconfig != "/tmp/flag-kubeconfig" {
		t.Fatalf("expected explicit kubeconfig from flag, got %q", cfg.kubeconfig)
	}
	if cfg.kubeconfigStatus != "/tmp/flag-kubeconfig" {
		t.Fatalf("expected status kubeconfig from flag, got %q", cfg.kubeconfigStatus)
	}
}

func TestRequireKubeconfigMissing(t *testing.T) {
	_, err := requireKubeconfig(cliConfig{}, func(string) string { return "" })

	if !errors.Is(err, errKubeconfigRequired) {
		t.Fatalf("expected required kubeconfig error, got %v", err)
	}
}

func TestFormatStatusLine(t *testing.T) {
	if got, want := formatStatusLine("abc", 5), "abc  "; got != want {
		t.Fatalf("expected padded status line %q, got %q", want, got)
	}
	if got, want := formatStatusLine("abcdef", 3), "abc"; got != want {
		t.Fatalf("expected truncated status line %q, got %q", want, got)
	}
	if got := formatStatusLine("abc", 0); got != "" {
		t.Fatalf("expected empty status line for zero width, got %q", got)
	}
}

func TestKubeconfigStatusLineIncludesNamespace(t *testing.T) {
	session := kube.NewSessionState("apps")
	cfg := cliConfig{kubeconfigStatus: "/tmp/config"}

	if got, want := kubeconfigStatusLine(cfg, session), " kube-prompt | kubeconfig: /tmp/config | namespace: apps "; got != want {
		t.Fatalf("expected status line %q, got %q", want, got)
	}

	session.SetNamespace("")
	if got, want := kubeconfigStatusLine(cfg, session), " kube-prompt | kubeconfig: /tmp/config | namespace: - "; got != want {
		t.Fatalf("expected status line %q, got %q", want, got)
	}
}

func TestStatusLineWriterAttachAndClose(t *testing.T) {
	base := &recordingPromptWriter{}
	writer := newStatusLineWriter(base, " kube-prompt | kubeconfig: /tmp/config ")
	writer.size = func() (rows, cols int, err error) {
		return 24, 40, nil
	}

	writer.Attach()

	out := base.String()
	if !strings.Contains(out, "\x1b[2;24r") {
		t.Fatalf("expected scroll region setup, got %q", out)
	}
	if !strings.Contains(out, " kube-prompt | kubeconfig: /tmp/config ") {
		t.Fatalf("expected status text, got %q", out)
	}
	if !strings.Contains(out, "<goto:2:1>") {
		t.Fatalf("expected cursor to move below status line, got %q", out)
	}

	base.Reset()
	if err := writer.Flush(); err != nil {
		t.Fatalf("expected flush to succeed, got %v", err)
	}
	if out := base.String(); !strings.Contains(out, " kube-prompt | kubeconfig: /tmp/config ") {
		t.Fatalf("expected status line refresh on flush, got %q", out)
	}

	base.Reset()
	writer.Close()
	out = base.String()
	if !strings.Contains(out, "\x1b[r") {
		t.Fatalf("expected scroll region reset, got %q", out)
	}
	if !strings.Contains(out, "<erase-line>") {
		t.Fatalf("expected status line cleanup, got %q", out)
	}
}

func TestStatusLineWriterRendersDynamicText(t *testing.T) {
	base := &recordingPromptWriter{}
	text := "first"
	writer := newDynamicStatusLineWriter(base, func() string {
		return text
	})
	writer.size = func() (rows, cols int, err error) {
		return 24, 40, nil
	}

	writer.Attach()
	base.Reset()
	text = "second"
	if err := writer.Flush(); err != nil {
		t.Fatalf("expected flush to succeed, got %v", err)
	}
	if out := base.String(); !strings.Contains(out, "second") {
		t.Fatalf("expected refreshed dynamic status text, got %q", out)
	}
}

type recordingPromptWriter struct {
	buf bytes.Buffer
}

func (w *recordingPromptWriter) String() string {
	return w.buf.String()
}

func (w *recordingPromptWriter) Reset() {
	w.buf.Reset()
}

func (w *recordingPromptWriter) WriteString(data string) {
	w.buf.WriteString(data)
}

func (w *recordingPromptWriter) WriteRaw(data []byte) {
	w.buf.Write(data)
}

func (w *recordingPromptWriter) Write(data []byte) {
	w.buf.Write(data)
}

func (w *recordingPromptWriter) WriteRawStr(data string) {
	w.WriteString(data)
}

func (w *recordingPromptWriter) WriteStr(data string) {
	w.WriteString(strings.ReplaceAll(data, "\x1b", "?"))
}

func (w *recordingPromptWriter) Flush() error {
	return nil
}

func (w *recordingPromptWriter) EraseScreen() {
	w.WriteString("<erase-screen>")
}

func (w *recordingPromptWriter) EraseUp() {
	w.WriteString("<erase-up>")
}

func (w *recordingPromptWriter) EraseDown() {
	w.WriteString("<erase-down>")
}

func (w *recordingPromptWriter) EraseStartOfLine() {
	w.WriteString("<erase-start-line>")
}

func (w *recordingPromptWriter) EraseEndOfLine() {
	w.WriteString("<erase-end-line>")
}

func (w *recordingPromptWriter) EraseLine() {
	w.WriteString("<erase-line>")
}

func (w *recordingPromptWriter) ShowCursor() {
	w.WriteString("<show-cursor>")
}

func (w *recordingPromptWriter) HideCursor() {
	w.WriteString("<hide-cursor>")
}

func (w *recordingPromptWriter) CursorGoTo(row, col int) {
	w.WriteString(fmt.Sprintf("<goto:%d:%d>", row, col))
}

func (w *recordingPromptWriter) CursorUp(n int) {
	w.WriteString(fmt.Sprintf("<up:%d>", n))
}

func (w *recordingPromptWriter) CursorDown(n int) {
	w.WriteString(fmt.Sprintf("<down:%d>", n))
}

func (w *recordingPromptWriter) CursorForward(n int) {
	w.WriteString(fmt.Sprintf("<forward:%d>", n))
}

func (w *recordingPromptWriter) CursorBackward(n int) {
	w.WriteString(fmt.Sprintf("<backward:%d>", n))
}

func (w *recordingPromptWriter) AskForCPR() {
	w.WriteString("<cpr>")
}

func (w *recordingPromptWriter) SaveCursor() {
	w.WriteString("<save>")
}

func (w *recordingPromptWriter) UnSaveCursor() {
	w.WriteString("<restore>")
}

func (w *recordingPromptWriter) ScrollDown() {
	w.WriteString("<scroll-down>")
}

func (w *recordingPromptWriter) ScrollUp() {
	w.WriteString("<scroll-up>")
}

func (w *recordingPromptWriter) SetTitle(title string) {
	w.WriteString(fmt.Sprintf("<title:%s>", title))
}

func (w *recordingPromptWriter) ClearTitle() {
	w.WriteString("<clear-title>")
}

func (w *recordingPromptWriter) SetColor(_, _ prompt.Color, _ bool) {
	w.WriteString("<color>")
}
