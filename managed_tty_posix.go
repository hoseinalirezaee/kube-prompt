//go:build !windows
// +build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/hoseinalirezaee/kube-prompt/prompt"
	"golang.org/x/term"
)

type managedTTY struct {
	file     *os.File
	oldState *term.State
}

func prepareManagedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interruptManagedCommand(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func openManagedTTY() (*managedTTY, error) {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "/dev/tty")
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = term.Restore(fd, oldState)
		_ = file.Close()
		return nil, err
	}
	return &managedTTY{file: file, oldState: oldState}, nil
}

func (t *managedTTY) Close() error {
	fd := int(t.file.Fd())
	_ = syscall.SetNonblock(fd, false)
	err := term.Restore(fd, t.oldState)
	if closeErr := t.file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func readManagedKeyEvents(tty *managedTTY, parser *prompt.KeyParser) []prompt.KeyEvent {
	buf := make([]byte, 1024)
	n, err := syscall.Read(int(tty.file.Fd()), buf)
	if n > 0 {
		return parser.Feed(buf[:n])
	}
	if err != nil && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
		return nil
	}
	return nil
}
