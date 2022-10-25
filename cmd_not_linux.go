//go:build !linux

package gosh

import (
	"k8s.io/klog/v2"
)

// A Cmd is a wrapper for building os/exec.Cmd's
type Cmd struct {
	*exec.Cmd
	RawCmd []string
	// BuilderError is set if a builder method like FileOut fails. Run() and Start() will return this error if set. Further builder methods will do nothing if this is set.
	BuilderError   error
	deferredBefore []func() error
	deferredAfter  []func() error
}

// Kill implements Commander
func (c *Cmd) Kill() error {
	if c.Cmd.Process == nil {
		return ErrNotStarted
	}
	klog.V(11).Infof("killing %d: %s %v", c.Cmd.Process.Pid, c.Path, c.Args)
	return c.Cmd.Process.Kill()
}
