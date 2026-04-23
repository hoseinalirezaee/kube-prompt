package kube

import "testing"

func TestKubectlCommandSetsKubeconfigEnv(t *testing.T) {
	cmd := kubectlCommand("get pods", "/tmp/kubeconfig")

	if cmd.Env == nil {
		t.Fatal("expected command environment to be set")
	}
	if !hasEnv(cmd.Env, "KUBECONFIG=/tmp/kubeconfig") {
		t.Fatalf("expected KUBECONFIG in command environment, got %v", cmd.Env)
	}
}

func TestKubectlCommandLeavesDefaultEnvWithoutKubeconfig(t *testing.T) {
	cmd := kubectlCommand("get pods", "")

	if cmd.Env != nil {
		t.Fatalf("expected nil command environment, got %v", cmd.Env)
	}
}

func hasEnv(env []string, want string) bool {
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}
