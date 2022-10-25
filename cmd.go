package gosh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"k8s.io/klog/v2"
	"os"
	"os/exec"
	"strings"
)

var (
	_ = Pipelineable(&Cmd{})
)

// Command returns a new Cmd
func Command(cmd ...string) *Cmd {
	if len(cmd) == 0 {
		return &Cmd{BuilderError: errors.New("Must have at least a command")}
	}
	return &Cmd{
		RawCmd:         cmd,
		Cmd:            exec.Command(cmd[0], cmd[1:]...),
		deferredBefore: make([]func() error, 0),
		deferredAfter:  make([]func() error, 0),
	}
}

// Shell is a convenience wrapper for Command("${SHELL}", "-c", script)
func Shell(script string) *Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return &Cmd{BuilderError: errors.New("SHELL is not set")}
	}
	return Command(shell, "-c", script)
}

// WithContext assigns a context to this command. WARNING: Because this requires re-creating the underlying os/exec.Cmd, this should ALWAYS be the first method called on a new Cmd
func (c *Cmd) WithContext(ctx context.Context) *Cmd {
	c.Cmd = exec.CommandContext(ctx, c.RawCmd[0], c.RawCmd[1:]...)
	return c
}

// WithParentEnv copies the current process's environment variables to this Cmd
func (c *Cmd) WithParentEnv() *Cmd {
	if c.BuilderError != nil {
		return c
	}
	c.Env = make([]string, len(os.Environ()))
	copy(c.Env, os.Environ())
	return c
}

// WithEnv sets one or more environment variables for this Cmd. Note that if you do not call WithParentEnv() first, the current process's variables will not be passed.
func (c *Cmd) WithEnv(env map[string]string) *Cmd {
	if c.BuilderError != nil {
		return c
	}
	envIndex := make(map[string]int, len(c.Cmd.Env))
	for ix, envLine := range c.Cmd.Env {
		split := strings.SplitN(envLine, "=", 2)
		envIndex[strings.ToLower(split[0])] = ix
	}
	for k, v := range env {
		line := fmt.Sprintf("%s=%s", k, v)
		ix, exists := envIndex[strings.ToLower(k)]
		if exists {
			c.Cmd.Env[ix] = line
		} else {
			c.Cmd.Env = append(c.Cmd.Env, line)
		}
	}
	return c
}

// WithParentEnvAnd is a convienence wrapper for WithParentEnv() then WithEnv()
func (c *Cmd) WithParentEnvAnd(env map[string]string) *Cmd {
	if c.BuilderError != nil {
		return c
	}
	return c.WithParentEnv().WithEnv(env)
}

// Run implements Commander
func (c *Cmd) Run() (err error) {
	if c.BuilderError != nil {
		return c.BuilderError
	}
	defer doDeferredAfter(&err, c.deferredAfter)
	err = doDeferredBefore(c.deferredBefore)
	if err != nil {
		return err
	}
	klog.V(11).Infof("starting: %s %v", c.Path, c.Args)
	err = c.Cmd.Run()
	klog.V(11).Infof("exited %d: %s %v", c.Cmd.ProcessState.ExitCode(), c.Path, c.Args)
	if err != nil {
		return
	}
	return nil
}

// Start implements Commander
func (c *Cmd) Start() error {
	if c.BuilderError != nil {
		return c.BuilderError
	}
	err := doDeferredBefore(c.deferredBefore)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	klog.V(11).Infof("%s %v &", c.Path, c.Args)
	klog.V(11).Infof("starting: %s %v", c.Path, c.Args)
	err = c.Cmd.Start()
	if err != nil {
		return err
	}

	klog.V(11).Infof("started %d: %s %v", c.Process.Pid, c.Path, c.Args)
	return nil
}

// Wait implements Commander
func (c *Cmd) Wait() (err error) {
	if c.Process == nil {
		return ErrNotStarted
	}
	defer doDeferredAfter(&err, c.deferredAfter)
	klog.V(11).Infof("waiting %d: %s %v", c.Cmd.Process.Pid, c.Path, c.Args)
	err = c.Cmd.Wait()
	klog.V(11).Infof("exited %d (%d): %s %v", c.Cmd.Process.Pid, c.Cmd.ProcessState.ExitCode(), c.Path, c.Args)
	if err != nil {
		return
	}
	return
}

// DeferBefore implements Pipelineable
func (c *Cmd) DeferBefore(f func() error) {
	c.deferredBefore = append(c.deferredBefore, f)
}

// DeferAfter implements Pipelineable
func (c *Cmd) DeferAfter(f func() error) {
	c.deferredAfter = append(c.deferredAfter, f)
}

// WithStreams applies a set of StreamSetters to this command
func (c *Cmd) WithStreams(fs ...StreamSetter) *Cmd {
	if c.BuilderError != nil {
		return c
	}
	for _, f := range fs {
		f(c)
	}
	return c
}

// SetStdin implements Pipelineable
func (c *Cmd) SetStdin(stdin io.Reader) error {
	if c.BuilderError != nil {
		return nil
	}
	c.Stdin = stdin
	return nil
}

// SetStdout implements Pipelineable
func (c *Cmd) SetStdout(stdout io.Writer) error {
	if c.BuilderError != nil {
		return nil
	}
	c.Stdout = stdout
	return nil
}

// SetStderr implements Pipelineable
func (c *Cmd) SetStderr(stderr io.Writer) error {
	if c.BuilderError != nil {
		return nil
	}
	c.Stderr = stderr
	return nil
}
