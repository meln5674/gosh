package gosh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"k8s.io/klog/v2"
	"os"
	"os/exec"
	"strings"
)

// A PipeSource is a function which can produce input for a command
type PipeSource func(io.Writer) error

// A PipeSink is a function which can process the output (including standard error) of a command
type PipeSink func(io.Reader) error

type StreamSetter func(Pipelineable) error

// SaveString returns a PipeSink which records the output in a string
func SaveString(str *string) PipeSink {
	return func(r io.Reader) error {
		out, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		*str = string(out)
		return nil
	}
}

// AppendString returns a PipeSink which appends the output to a string
func AppendString(str *string) PipeSink {
	return func(r io.Reader) error {
		out, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		*str += string(out)
		return nil
	}
}

// SaveBytes returns a PipeSink which records the output in a byte slice
func SaveBytes(bytes *[]byte) PipeSink {
	return func(r io.Reader) error {
		out, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		*bytes = out
		return nil
	}
}

// AppendBytes returns a PipeSink which appendss the output to a byte slice
func AppendBytes(bytes *[]byte) PipeSink {
	return func(r io.Reader) error {
		out, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		*bytes = append(*bytes, out...)
		return nil
	}
}

// A Commander can be ran, started, killed, and waited for like a process
type Commander interface {
	// Start runs the task in the background and returns immediately. Start should not return an error if the underlying entity failed, but only if starting it failed.
	Start() error
	// Wait waits for a task to finish after it has been Start()'ed. If the Commander is Kill()'ed, then Wait must return an error indicating such.
	Wait() error
	// Kill forcefully terminates the task. Kill should not be used with Run(), as this behavior is not specified, and should only be used with Start(). A Kill'ed Commander should still be Wait()'ed. Kill shoult not return the error caused by killing the underlying entity, but instead, only return an error if the killing itself failed.
	Kill() error
	// Run starts the task in the foreground and waits for it to finish
	Run() error
}

// A Pipelineable is a Commander with a single set of standard files, meaning it is something which can be used in a pipeline
type Pipelineable interface {
	Commander
	SetStdin(io.Reader) error
	SetStdout(io.Writer) error
	SetStderr(io.Writer) error
	// DeferBefore adds a function to be called prior to actually starting the command. If it fails, then the command will return the error from Start() or Run(). Functions are called in the order they are added, and functions after the first failed function are not called.
	DeferBefore(func() error)
	// DeferAfter adds a function to called once the command finishes. If any fail, the command will return the errors from Run() or Wait(). Commands are called in the order they are added, but all functions will be called. Functions will still be called if a DeferBefore function fails, so if adding a DeferBefore+DeferAfter pair, the DeferAfter function must check if the DeferBefore function was called.
	DeferAfter(func() error)
}

var (
	// ErrorKilled is returned from Commander.Wait() if it is killed when no actual process is executing, otherwise, it will return whatever underlying error results from that process being killed
	ErrorKilled = errors.New("Killed")

	// ErrorNotStarted is returned from Commander.Wait() and Commander.Kill() if Start() was never called
	ErrorNotStarted = errors.New("Not Started")

	// ErrorAlreadyStarted is returned from Commander.Start() or Commander.Run() if either were already called
	ErrorAlreadyStarted = errors.New("Already Started")
)

// A MultiProcessError indicates one or more proceses, either in serial or parallel, failed
type MultiProcessError struct {
	// Errors are the errors that occurred. Order is not guaranteed.
	Errors []error
}

// Error implements error
func (e *MultiProcessError) Error() string {
	msg := strings.Builder{}
	msg.WriteString("One or more processes failed: (")
	for _, err := range e.Errors {
		msg.WriteString(err.Error())
		msg.WriteString(", ")
	}
	msg.WriteString(")")
	return msg.String()
}

type DeferredError struct {
	*MultiProcessError
}

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

// ForwardIn sets a a command's stdin to the current process's stdin
func ForwardIn(p Pipelineable) error {
	return p.SetStdin(os.Stdin)
}

// ForwardOut sets a a command's stdout to the current process's stdout
func ForwardOut(p Pipelineable) error {
	return p.SetStdout(os.Stdout)
}

// ForwardErr sets a a command's stderr to the current process's stderr
func ForwardErr(p Pipelineable) error {
	return p.SetStderr(os.Stderr)
}

// ForwardInOut does both ForwardIn and ForwardOut
func ForwardInOut(p Pipelineable) error {
	err := ForwardIn(p)
	if err != nil {
		return err
	}
	return ForwardOut(p)
}

// ForwardInErr does both ForwardIn and ForwardErr
func ForwardInErr(p Pipelineable) error {
	err := ForwardIn(p)
	if err != nil {
		return err
	}
	return ForwardErr(p)
}

// ForwardOutErr does both ForwardOut and ForwardErr
func ForwardOutErr(p Pipelineable) error {
	err := ForwardOut(p)
	if err != nil {
		return err
	}
	return ForwardErr(p)
}

// ForwardAll does ForwardIn, ForwardOut, and ForwardErr
func ForwardAll(p Pipelineable) error {
	err := ForwardIn(p)
	if err != nil {
		return err
	}
	return ForwardOutErr(p)
}

var (
	_ = StreamSetter(ForwardIn)
	_ = StreamSetter(ForwardOut)
	_ = StreamSetter(ForwardErr)
	_ = StreamSetter(ForwardInOut)
	_ = StreamSetter(ForwardOutErr)
	_ = StreamSetter(ForwardInErr)
	_ = StreamSetter(ForwardAll)
)

// FuncIn sets a function to pipe into the the stdin of this Cmd. You can think of this as pipeping your program directly into this Cmd's stdin. If this processing function fails, it will be returned from Run() or Wait() as if it were another process in the pipeline
func FuncIn(handler PipeSource) StreamSetter {
	return func(p Pipelineable) error {
		errChan := make(chan error)
		started := false
		var reader, writer *os.File
		p.DeferBefore(func() error {
			var err error
			reader, writer, err = os.Pipe()
			if err != nil {
				return err
			}
			err = p.SetStdin(reader)
			if err != nil {
				reader.Close()
				writer.Close()
				return err
			}
			go func() {
				err := handler(writer)
				writer.Close()
				errChan <- err
			}()
			started = true
			return nil
		})
		p.DeferAfter(func() error {
			if started {
				reader.Close()
				return <-errChan
			}
			return nil
		})
		return nil
	}
}

// FuncOut sets a function to feed the stdout of this Cmd into. You can think of this as piping this Cmd's stdout back into your program. If this processing function fails, it will be returned from Run() or Wait() as if it were another process in a pipeline.
func FuncOut(handler PipeSink) StreamSetter {
	return func(p Pipelineable) error {
		errChan := make(chan error)
		started := false
		var reader, writer *os.File
		p.DeferBefore(func() error {
			var err error
			reader, writer, err = os.Pipe()
			if err != nil {
				return err
			}
			err = p.SetStdout(writer)
			if err != nil {
				reader.Close()
				writer.Close()
				return err
			}
			go func() {
				defer reader.Close()
				errChan <- handler(reader)
			}()
			started = true
			return nil
		})
		p.DeferAfter(func() error {
			if started {
				writer.Close()
				return <-errChan
			}
			return nil
		})
		return nil
	}
}

// FuncErr is the same as FuncOut except it processes stderr
func FuncErr(handler PipeSink) StreamSetter {
	return func(p Pipelineable) error {
		errChan := make(chan error)
		started := false
		var reader, writer *os.File
		p.DeferBefore(func() error {
			var err error
			reader, writer, err = os.Pipe()
			if err != nil {
				return err
			}
			err = p.SetStderr(writer)
			if err != nil {
				reader.Close()
				writer.Close()
				return err
			}
			go func() {
				defer reader.Close()
				errChan <- handler(reader)
			}()
			started = true
			return nil
		})
		p.DeferAfter(func() error {
			if started {
				writer.Close()
				return <-errChan
			}
			return nil
		})
		return nil
	}
}

// StringIn sets a literal string to be provided as stdin to this Cmd.
func StringIn(in string) StreamSetter {
	return BytesIn([]byte(in))
}

// BytesIn sets a literal byte slice to be provided as stdin to this Cmd.
func BytesIn(in []byte) StreamSetter {
	return FuncIn(func(w io.Writer) error {

		_, err := w.Write(in)
		return err
	})
}

// FileIn sets the path of a file whose contents are to be to redirected to this Cmd's stdin. The file is not opened when FileIn is called, but instead when Run() or Start() is called.
func FileIn(path string) StreamSetter {
	return func(p Pipelineable) error {
		var f *os.File
		p.DeferBefore(func() error {
			var err error
			f, err = os.Open(path)
			if err != nil {
				return err
			}
			err = p.SetStdin(f)
			if err != nil {
				return err
			}
			return nil
		})
		p.DeferAfter(func() error {
			if f == nil {
				return nil
			}
			return f.Close()
		})
		return nil
	}
}

// FileOut sets the path of a file to redirect this Cmd's stdout to. The file is not opened when FileOut is called, but instead when Run() or Start() is called.
func FileOut(path string) StreamSetter {
	return func(p Pipelineable) error {
		var f *os.File
		p.DeferBefore(func() error {
			var err error
			f, err = os.Create(path)
			if err != nil {
				return err
			}
			err = p.SetStdout(f)
			if err != nil {
				return err
			}
			return nil
		})
		p.DeferAfter(func() error {
			if f == nil {
				return nil
			}
			return f.Close()
		})
		return nil
	}
}

// FileErr sets the path of a file to redirect this Cmd's stderr to. The file is not opened when FileErr is called, but instead when Run() or Start() is called.
func FileErr(path string) StreamSetter {
	return func(p Pipelineable) error {
		var f *os.File
		p.DeferBefore(func() error {
			var err error
			f, err = os.Create(path)
			if err != nil {
				return err
			}
			err = p.SetStderr(f)
			if err != nil {
				return err
			}
			return nil
		})
		p.DeferAfter(func() error {
			if f == nil {
				return nil
			}
			return f.Close()
		})
		return nil
	}
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

func doDeferredAfter(retErr *error, deferredAfter []func() error) {
	origErr := *retErr
	errs := make([]error, 0, len(deferredAfter))
	if origErr != nil {
		errs = append(errs, origErr)
	}
	for _, f := range deferredAfter {
		err := f()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if (origErr == nil && len(errs) > 0) || (origErr != nil && len(errs) > 1) {
		*retErr = &MultiProcessError{Errors: errs}
	}
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

type ixerr struct {
	ix  int
	err error
}

// Wait implements Commander
func (p *PipelineCmd) Wait() error {
	errs := make([]error, 0, len(p.Cmds))
	// Iterating in reverse because if a downstream process, its writer may be blocking forever on its stdout
	// If that's the case, then we attempt to kill each process upstream from that because there's no point in letting them finish
	errChan := make(chan ixerr)
	sem := make(chan struct{})
	go func() {
		for _ = range p.Cmds {
			_ = <-sem
		}
		close(errChan)
	}()
	for ix, cmd := range p.Cmds {
		go func(ix int, cmd Pipelineable) {
			defer func() {
				sem <- struct{}{}
			}()
			err := cmd.Wait()
			if ix > 0 {
				p.InPipes[ix-1].Close()
			}
			if ix < len(p.Cmds)-1 {
				p.OutPipes[ix].Close()
			}
			if err != nil {
				errChan <- ixerr{ix: ix, err: err}
			}
		}(ix, cmd)
	}
	for err := range errChan {
		errs = append(errs, err.err)
	}
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// Kill implements Commander
func (p *PipelineCmd) Kill() error {
	errs := make([]error, 0, len(p.Cmds))
	for _, cmd := range p.Cmds {
		err := cmd.Kill()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

type sequenceEvent struct {
	kill *struct{}
	err  *ixerr
	next *int
}

// A SequenceGate indicates to a Sequence what to do when a command finishes, and has a chance to modify the final error
type SequenceGate func(s *SequenceCmd, ix int, err error, killed bool) (continue_ bool, finalErr error)

// A SequenceCmd executes commands in order, one at a time,
// stopping when either there are no more or a gate function indicates to stop early.
// Kill() will stop a SequenceCmd regardless of the output of the gate function, but the gate function can still mutate the final error.
type SequenceCmd struct {
	Cmds []Commander
	Gate SequenceGate
	// CmdErrors records the errors returned by the corresponding Cmd's since a SequenceCmd does not necessarily stop when a command fails
	CmdErrors      []error
	BuilderError   error
	kill           chan struct{}
	result         chan error
	deferredBefore []func() error
	deferredAfter  []func() error
}

var (
	_ = Pipelineable(&SequenceCmd{})
)

func Sequence(gate SequenceGate, cmds ...Commander) *SequenceCmd {
	return &SequenceCmd{Gate: gate, Cmds: cmds, CmdErrors: make([]error, 0, len(cmds))}
}

func (s *SequenceCmd) Run() error {
	if s.BuilderError != nil {
		return s.BuilderError
	}

	var err error
	var continue_ bool
	defer doDeferredAfter(&err, s.deferredAfter)
	err = doDeferredBefore(s.deferredBefore)
	if err != nil {
		return err
	}
	for ix, cmd := range s.Cmds {
		err = cmd.Run()
		if err != nil {
			s.CmdErrors = append(s.CmdErrors, err)
		}
		continue_, err = s.Gate(s, ix, err, false)
		if !continue_ {
			break
		}
	}
	return err

}

func (s *SequenceCmd) Start() error {
	if s.BuilderError != nil {
		return s.BuilderError
	}

	err := doDeferredBefore(s.deferredBefore)
	if err != nil {
		return err
	}
	s.kill = make(chan struct{})
	s.result = make(chan error)
	go func() {
		var err error
		var continue_ bool
	loop:
		for ix, cmd := range s.Cmds {
			select {
			case _ = <-s.kill:
				err = ErrorKilled
				_, err = s.Gate(s, ix, err, true)
				break loop
			default:
				err = cmd.Run()
				if err != nil {
					s.CmdErrors = append(s.CmdErrors, err)
				}
				continue_, err = s.Gate(s, ix, err, false)
				if !continue_ {
					break loop
				}
			}
		}
		s.result <- err
	}()
	return nil
}

func (s *SequenceCmd) Kill() error {
	s.kill <- struct{}{}
	return nil
}

func (s *SequenceCmd) Wait() (err error) {
	defer doDeferredAfter(&err, s.deferredAfter)
	err = <-s.result
	return
}

func (s *SequenceCmd) SetStdin(r io.Reader) error {
	for _, cmd := range s.Cmds {
		p, ok := cmd.(Pipelineable)
		if !ok {
			continue
		}
		err := p.SetStdin(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SequenceCmd) SetStdout(w io.Writer) error {
	for _, cmd := range s.Cmds {
		p, ok := cmd.(Pipelineable)
		if !ok {
			continue
		}
		err := p.SetStdout(w)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SequenceCmd) SetStderr(w io.Writer) error {
	for _, cmd := range s.Cmds {
		p, ok := cmd.(Pipelineable)
		if !ok {
			continue
		}
		err := p.SetStderr(w)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SequenceCmd) DeferBefore(f func() error) {
	s.deferredBefore = append(s.deferredBefore, f)
}

func (s *SequenceCmd) DeferAfter(f func() error) {
	s.deferredAfter = append(s.deferredAfter, f)
}

func (s *SequenceCmd) WithStreams(fs ...StreamSetter) *SequenceCmd {
	if s.BuilderError != nil {
		return s
	}
	for _, f := range fs {
		err := f(s)
		if err != nil {
			s.BuilderError = err
			return s
		}
	}
	return s
}

func And(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continue_ bool, finalError error) {
			if ix == len(s.Cmds)-1 {
				errs := make([]error, 0, len(s.CmdErrors))
				for _, cmdErr := range s.CmdErrors {
					errs = append(errs, cmdErr)
				}
				if len(errs) != 0 {
					return true, &MultiProcessError{Errors: errs}
				}
			}
			if err != nil {
				return false, err
			}
			return true, err
		},
		cmds...,
	)
}

func Or(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continue_ bool, finalError error) {
			if ix == len(s.Cmds)-1 {
				errs := make([]error, 0, len(s.CmdErrors))
				for _, cmdErr := range s.CmdErrors {
					errs = append(errs, cmdErr)
				}
				if len(errs) != 0 {
					return true, &MultiProcessError{Errors: errs}
				}
			}
			if err != nil {
				return true, err
			}
			return false, err
		},
		cmds...,
	)
}

func Then(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continue_ bool, finalError error) {
			return true, err
		},
		cmds...,
	)
}

// A FanOutCmd runs a set of commands in parallel, with some limit as to how many can run concurrently
type FanOutCmd struct {
	Cmds           []Commander
	MaxConcurrency int
	sem            chan struct{}
	errChan        chan error
	kill           chan struct{}
	BuilderError   error
}

var (
	_ = Commander(&FanOutCmd{})
)

// FanOut creates a new FanOutCmd that lets all commands run at the same time
func FanOut(cmds ...Commander) *FanOutCmd {
	return &FanOutCmd{Cmds: cmds, MaxConcurrency: len(cmds)}
}

// WithMaxConcurrency sets the maximum number of commans that can run at the same time
func (f *FanOutCmd) WithMaxConcurrency(max int) *FanOutCmd {
	if max < 1 {
		f.BuilderError = errors.New("Max Concurrency must be Positive")
		return f
	}
	f.MaxConcurrency = max
	return f
}

func (f *FanOutCmd) Run() error {
	if f.BuilderError != nil {
		return f.BuilderError
	}
	err := f.Start()
	if err != nil {
		return err
	}
	err = f.Wait()
	if err != nil {
		return err
	}
	return nil
}

// Start implements Commander
func (f *FanOutCmd) Start() error {
	if f.BuilderError != nil {
		return f.BuilderError
	}
	cmdChan := make(chan Commander)
	f.errChan = make(chan error)
	f.sem = make(chan struct{})
	go func() {
		for _, cmd := range f.Cmds {
			cmdChan <- cmd
		}
		close(cmdChan)
		klog.V(0).Info("all commands pushed")
	}()
	for ix := 0; ix < f.MaxConcurrency; ix++ {
		go func() {
			klog.V(0).Info("fanout started")
			defer func() {
				f.sem <- struct{}{}
				klog.V(0).Info("sem++")
				klog.V(0).Info("fanout finished")
			}()
			for {
				select {
				case cmd := <-cmdChan:
					f.errChan <- cmd.Run()
					klog.V(0).Info("Wrote err")
				case _ = <-f.kill:
					klog.V(0).Info("Got kill signal, emptying cmdChan")
					for _ = range cmdChan {
					}
				}
			}
		}()
	}
	go func() {
		for ix := 0; ix < f.MaxConcurrency; ix++ {
			_ = <-f.sem
			klog.V(0).Info("sem--")
		}
		klog.V(0).Info("all fanouts finished")
		close(f.errChan)
	}()
	return nil
}

// Wait implements Commander
func (f *FanOutCmd) Wait() error {
	errs := make([]error, 0, len(f.Cmds))
	for err := range f.errChan {
		klog.V(0).Info("Read err")
		if err != nil {
			errs = append(errs, err)
		}
	}
	klog.V(0).Info("all errors recorded")
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// Kill implements Commander
func (f *FanOutCmd) Kill() error {
	f.kill <- struct{}{}
	errs := make([]error, 0, len(f.Cmds))
	for _, cmd := range f.Cmds {
		err := cmd.Kill()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// A PipelineCmd is two or more processes, run in parallel, which feed the stdout of each process to the next process's stdin like a standard shell pipe. Pipelines can accept anything which act like commands but also have standard in, out, and error to manipulate. Pipelines can be fed into other pipelines.
type PipelineCmd struct {
	// Cmds are the commands to run, in order
	Cmds []Pipelineable
	// InPipes are the read side of the pipes
	InPipes []*os.File
	// OutPipes are the write side of the pipes
	OutPipes []*os.File
	// BuilderError is set if a builder method like FileOut fails. Run() and Start() will return this error if set. Further builder methods will do nothing if this is set.
	BuilderError error
}

var (
	_ = Pipelineable(&PipelineCmd{})
)

// Pipeline creates a new pipeline
func Pipeline(cmd ...Pipelineable) *PipelineCmd {
	if len(cmd) < 2 {
		return &PipelineCmd{BuilderError: fmt.Errorf("Need at least two commands for a pipeline")}
	}
	prevCmd := cmd[0]
	inPipes := make([]*os.File, 0, len(cmd)-1)
	outPipes := make([]*os.File, 0, len(cmd)-1)
	cleanupPipes := func(endIx int) {
		for ix := 0; ix < endIx; ix++ {
			inPipes[ix].Close()
			outPipes[ix].Close()
		}
	}
	for ix, nextCmd := range cmd[1:] {
		reader, writer, err := os.Pipe()
		if err != nil {
			cleanupPipes(ix)
			return &PipelineCmd{BuilderError: err}
		}
		inPipes = append(inPipes, reader)
		outPipes = append(outPipes, writer)
		err = prevCmd.SetStdout(writer)
		if err != nil {
			cleanupPipes(ix + 1)
			return &PipelineCmd{BuilderError: err}
		}
		err = nextCmd.SetStdin(reader)
		if err != nil {
			cleanupPipes(ix + 1)
			return &PipelineCmd{BuilderError: err}
		}
	}
	return &PipelineCmd{Cmds: cmd, InPipes: inPipes, OutPipes: outPipes}
}

// Run implements Commander
func (p *PipelineCmd) Run() error {
	if p.BuilderError != nil {
		return p.BuilderError
	}
	err := p.Start()
	if err != nil {
		return err
	}
	err = p.Wait()
	klog.V(0).Info("pipeline finished")
	if err != nil {
		return err
	}
	return nil
}

// Start implements Commander
func (p *PipelineCmd) Start() error {
	if p.BuilderError != nil {
		return p.BuilderError
	}
	for ix, cmd := range p.Cmds {
		err := cmd.Start()
		if err != nil {
			for ix2 := 0; ix2 < ix; ix2++ {
				p.Cmds[ix2].Kill()
			}
			return err
		}
	}
	return nil
}

func (p *PipelineCmd) DeferBefore(f func() error) {
	p.Cmds[0].DeferBefore(f)
}

func (p *PipelineCmd) DeferAfter(f func() error) {
	p.Cmds[len(p.Cmds)-1].DeferAfter(f)
}

func (p *PipelineCmd) WithStreams(fs ...StreamSetter) *PipelineCmd {
	if p.BuilderError != nil {
		return p
	}
	for _, f := range fs {
		f(p)
	}
	return p
}

// StdStdin implements Pipelineable by setting stdin for the first command in the pipeline
func (p *PipelineCmd) SetStdin(stdin io.Reader) error {
	if p.BuilderError != nil {
		return nil
	}
	return p.Cmds[0].SetStdin(stdin)
}

// StdStdout implements Pipelineable by setting stdout for the last command in the pipeline
func (p *PipelineCmd) SetStdout(stdout io.Writer) error {
	if p.BuilderError != nil {
		return nil
	}
	return p.Cmds[len(p.Cmds)-1].SetStdout(stdout)
}

// StdStderr implements Pipelineable by setting stderr for all commands in the pipeline
func (p *PipelineCmd) SetStderr(stderr io.Writer) error {
	if p.BuilderError != nil {
		return nil
	}
	for _, cmd := range p.Cmds {
		err := cmd.SetStderr(stderr)
		if err != nil {
			return err
		}
	}
	return nil
}
