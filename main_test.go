package main

import (
	"bytes"
	"strings"
	"testing"
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

	cfg, ok := parseCLI([]string{"--kubeconfig", "/tmp/kubeconfig"}, &stdout, &stderr)

	if !ok {
		t.Fatalf("expected parse to succeed, stderr %q", stderr.String())
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.kubeconfig != "/tmp/kubeconfig" {
		t.Fatalf("expected kubeconfig path, got %q", cfg.kubeconfig)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, stdout %q stderr %q", stdout.String(), stderr.String())
	}
}
