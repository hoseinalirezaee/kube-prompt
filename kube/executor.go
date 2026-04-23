package kube

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hoseinalirezaee/kube-prompt/internal/debug"
)

func NewExecutor(kubeconfig string) func(string) {
	return func(s string) {
		execute(s, kubeconfig)
	}
}

func Executor(s string) {
	execute(s, "")
}

func execute(s, kubeconfig string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	} else if s == "quit" || s == "exit" {
		fmt.Println("Bye!")
		os.Exit(0)
		return
	}

	cmd := kubectlCommand(s, kubeconfig)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Got error: %s\n", err.Error())
	}
}

func ExecuteAndGetResult(s string) string {
	return ExecuteAndGetResultWithKubeconfig(s, "")
}

func ExecuteAndGetResultWithKubeconfig(s, kubeconfig string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		debug.Log("you need to pass the something arguments")
		return ""
	}

	out := &bytes.Buffer{}
	cmd := kubectlCommand(s, kubeconfig)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		debug.Log(err.Error())
		return ""
	}
	r := string(out.Bytes())
	return r
}

func kubectlCommand(s, kubeconfig string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", "kubectl "+s)
	if kubeconfig != "" {
		cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	}
	return cmd
}
