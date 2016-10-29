package oslib

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
)

// TODO: Find a way to make this safer/more secure.
func BashCmd(cmdStr string) *exec.Cmd {
	return exec.Command("bash", "-c", cmdStr)
}

// TODO: Find a way to make this safer/more secure.
func BashCmdf(cmdStr string, args ...interface{}) *exec.Cmd {
	return BashCmd(fmt.Sprintf(cmdStr, args...))
}

func AttachCmd(cmd *exec.Cmd, stdout io.Writer, stderr io.Writer, stdin io.Reader) (*sync.WaitGroup, error) {
	var wg sync.WaitGroup
	wg.Add(2)

	stdinIn, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutOut, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrOut, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	go func() {
		io.Copy(stdinIn, stdin)
		stdinIn.Close()
	}()
	go func() {
		io.Copy(stdout, stdoutOut)
		wg.Done()
	}()
	go func() {
		io.Copy(stderr, stderrOut)
		wg.Done()
	}()

	return &wg, nil
}

func ExitStatus(err error) (uint32, error) {
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// There is no platform independent way to retrieve
			// the exit code, but the following will work on Unix.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				return uint32(status.ExitStatus()), nil
			}
		}
		return 0, err
	}
	return 0, nil
}
