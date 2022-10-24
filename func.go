package gosh

import (
	"context"
	"errors"
	"io"
	"os"
)

// A Func is a function which can be used to implement a go-native Commander/Pipelineable.
// If the function cannot start before calling any blocking (or other long-running) functions, it should return err.
// If any blocking calls would be called, instead you should still return a nil err, and send any potential errors from those blocking calls to the provided "done" channel.
// Functions are expected to close the done channel before returning, e.g. with a defer statement.
// Functions are expected to close stdout and stderr if not returning an initial error
type Func func(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, done chan error) error

// FuncCmd is a wrapper for the state of a Func which implements Commander and Pipelineable. This can be used to, for example, insert native go-code in between two Pipelines
type FuncCmd struct {
	Func           Func
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	ctx            context.Context
	kill           context.CancelFunc
	done           chan error
	deferredBefore []func() error
	deferredAfter  []func() error
}

var (
	_ = Pipelineable(&FuncCmd{})
)

// FromFunc produces a Commander/Pipelineable from a compliant function
func FromFunc(parentCtx context.Context, f Func) *FuncCmd {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		// This shouldn't ever happen, right?
		panic(err)
	}
	cmd := &FuncCmd{
		Func:   f,
		Stdin:  devNull,
		Stdout: devNull,
		Stderr: devNull,
	}
	cmd.ctx, cmd.kill = context.WithCancel(parentCtx)
	return cmd
}

// Start implements Commander
func (f *FuncCmd) Start() error {
	if f.done != nil {
		return ErrAlreadyStarted
	}
	err := doDeferredBefore(f.deferredBefore)
	if err != nil {
		return err
	}
	f.done = make(chan error)
	err = f.Func(f.ctx, f.Stdin, f.Stdout, f.Stderr, f.done)
	if err != nil {
		return err
	}
	return nil
}

// Wait implements Commander
func (f *FuncCmd) Wait() error {
	err := <-f.done
	if errors.Is(err, context.Canceled) {
		return ErrKilled
	}
	return err
}

// Kill implements Commander
func (f *FuncCmd) Kill() error {
	if f.done == nil {
		return ErrNotStarted
	}
	f.kill()
	return nil
}

// Run implements Commander
func (f *FuncCmd) Run() error {
	if f.done != nil {
		return ErrAlreadyStarted
	}
	var err error
	err = f.Start()
	if err != nil {
		return err
	}
	err = f.Wait()
	if err != nil {
		return err
	}
	return nil
}

// DeferBefore implements Pipelineable
func (f *FuncCmd) DeferBefore(fn func() error) {
	f.deferredBefore = append(f.deferredBefore, fn)
}

// DeferAfter implements Pipelineable
func (f *FuncCmd) DeferAfter(fn func() error) {
	f.deferredAfter = append(f.deferredAfter, fn)
}

// SetStdin implements Pipelineable
func (f *FuncCmd) SetStdin(stdin io.Reader) error {
	f.Stdin = stdin
	return nil
}

// SetStdout implements Pipelineable
func (f *FuncCmd) SetStdout(stdout io.Writer) error {
	f.Stdout = stdout
	return nil
}

// SetStderr implements Pipelineable
func (f *FuncCmd) SetStderr(stderr io.Writer) error {
	f.Stderr = stderr
	return nil
}

// WithStreams applies a set of StreamSetters to this command
func (f *FuncCmd) WithStreams(fs ...StreamSetter) *FuncCmd {
	for _, fn := range fs {
		fn(f)
	}
	return f
}
