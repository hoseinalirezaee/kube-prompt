//go:build windows
// +build windows

package main

import (
	"errors"
	"os"
	"os/exec"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
)

type managedTTY struct{}

func prepareManagedCommand(_ *exec.Cmd) {}

func interruptManagedCommand(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func openManagedTTY() (*managedTTY, error) {
	return nil, errors.New("managed output tty is not supported on windows")
}

func (t *managedTTY) Close() error {
	return nil
}

func readManagedKeyEvents(_ *managedTTY, _ *prompt.KeyParser) []prompt.KeyEvent {
	return nil
}
