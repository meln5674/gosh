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

// A Cmd is a wrapper for building os/exec.Cmd's
type Cmd struct {
	*exec.Cmd
	RawCmd []string
	// BuilderError is set if a builder method like FileOut fails. Run() and Start() will return this error if set. Further builder methods will do nothing if this is set.
	BuilderError   error
	deferredBefore []func() error
	deferredAfter  []func() error
}

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

// WithParentEnvAndCmd is a convienence wrapper for WithParentEnv() then WithEnv()
func (c *Cmd) WithParentEnvAnd(env map[string]string) *Cmd {
	if c.BuilderError != nil {
		return c
	}
	return c.WithParentEnv().WithEnv(env)
}

func doDeferredBefore(deferredBefore []func() error) error {
	for _, f := range deferredBefore {
		err := f()
		if err != nil {
			return err
		}
	}
	return nil
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
	klog.V(0).Infof("%s %v", c.Path, c.Args)
	err = c.Cmd.Run()
	klog.V(0).Infof("exited %d: %s %v", c.Cmd.ProcessState.ExitCode(), c.Path, c.Args)
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
	klog.V(0).Infof("%s %v &", c.Path, c.Args)
	return c.Cmd.Start()
}

// Wait implements Commander
func (c *Cmd) Wait() (err error) {
	defer doDeferredAfter(&err, c.deferredAfter)
	klog.V(0).Info("waiting: %s %v", c.Path, c.Args)
	err = c.Cmd.Wait()
	klog.V(0).Info("exited %d: %s %v", c.Cmd.ProcessState.ExitCode(), c.Path, c.Args)
	if err != nil {
		return
	}
	return
}

// Kill implements Commander
func (c *Cmd) Kill() error {
	return c.Cmd.Process.Kill()
}

func (c *Cmd) DeferBefore(f func() error) {
	c.deferredBefore = append(c.deferredBefore, f)
}

func (c *Cmd) DeferAfter(f func() error) {
	c.deferredAfter = append(c.deferredAfter, f)
}

func (c *Cmd) WithStreams(fs ...StreamSetter) *Cmd {
	if c.BuilderError != nil {
		return c
	}
	for _, f := range fs {
		f(c)
	}
	return c
}

// StdStdin implements Pipelineable
func (c *Cmd) SetStdin(stdin io.Reader) error {
	if c.BuilderError != nil {
		return nil
	}
	c.Stdin = stdin
	return nil
}

// StdStdout implements Pipelineable
func (c *Cmd) SetStdout(stdout io.Writer) error {
	if c.BuilderError != nil {
		return nil
	}
	c.Stdout = stdout
	return nil
}

// StdStderr implements Pipelineable
func (c *Cmd) SetStderr(stderr io.Writer) error {
	if c.BuilderError != nil {
		return nil
	}
	c.Stderr = stderr
	return nil
}
