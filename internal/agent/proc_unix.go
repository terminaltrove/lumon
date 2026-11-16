//go:build !windows

package agent

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// alive reports whether process pid exists. EPERM means it does, under
// another user.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Detach starts a program in its own session with no terminal, so it outlives
// the hook that started it.
func Detach(name string, args ...string) error { return DetachWith(nil, name, args...) }

// DetachWith is Detach with input written to the program's stdin. Its output
// goes nowhere, so an agent that waits for the hook's output to close, or reads
// it, sees none.
func DetachWith(input []byte, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = null, null, null
	var pipe io.WriteCloser
	if input != nil {
		cmd.Stdin = nil
		if pipe, err = cmd.StdinPipe(); err != nil {
			return err
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if pipe != nil {
		// Small enough to fit the pipe's buffer, so this never blocks.
		pipe.Write(input)
		pipe.Close()
	}
	return cmd.Process.Release()
}
