//go:build !linux

package gosh

import (
	"fmt"
	"os/exec"

	"github.com/go-logr/logr"
)

// A Cmd is a wrapper for building os/exec.Cmd's
type Cmd struct {
	*exec.Cmd
	RawCmd []string
	// BuilderError is set if a builder method like FileOut fails. Run() and Start() will return this error if set. Further builder methods will do nothing if this is set.
	BuilderError   error
	Log            logr.Logger
	deferredBefore []func() error
	deferredAfter  []func() error
}

// Kill implements Commander
func (c *Cmd) Kill() error {
	if c.Cmd.Process == nil {
		return ErrNotStarted
	}
	c.log().V(CommandLogLevel).Info(fmt.Sprintf("killing: %v", c.AsShellArgs()), "pid", c.Process.Pid)
	return c.Cmd.Process.Kill()
}
