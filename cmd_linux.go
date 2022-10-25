//go:build linux

package gosh

import (
	"k8s.io/klog/v2"
	"os/exec"
	"syscall"
)

// A Cmd is a wrapper for building os/exec.Cmd's
type Cmd struct {
	*exec.Cmd
	RawCmd []string
	// BuilderError is set if a builder method like FileOut fails. Run() and Start() will return this error if set. Further builder methods will do nothing if this is set.
	BuilderError    error
	UseProcessGroup bool
	deferredBefore  []func() error
	deferredAfter   []func() error
}

// UsingProcessGroup marks this Cmd to create a new process group.
// If this is not done, calling Kill() will not kill any subprocesses this Cmd spawns,
// causing Wait() to never return.
// This is not necessary if you are not planning on Kill()'ing this Cmd, or are 100% certain it
// will not spawn any subprocesses.
func (c *Cmd) UsingProcessGroup() *Cmd {
	c.UseProcessGroup = true
	c.Cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return c
}

// Kill implements Commander
func (c *Cmd) Kill() error {
	if c.Cmd.Process == nil {
		return ErrNotStarted
	}

	klog.V(11).Infof("killing %d: %s %v", c.Cmd.Process.Pid, c.Path, c.Args)
	if c.UseProcessGroup {
		pgid, err := syscall.Getpgid(c.Process.Pid)
		if err != nil {
			return err
		}
		err = syscall.Kill(-pgid, syscall.SIGTERM)
		if err != nil {
			return err
		}
	}
	return c.Cmd.Process.Signal(syscall.SIGTERM)
}
