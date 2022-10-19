package command

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

// A PipeProcessor is a function which can process the output of a command
type PipeProcessor func(io.Reader) error

// A Commander can be ran, started, killed, and waited for like a process
type Commander interface {
	// Start runs the task in the background and returns immediately
	Start() error
	// Wait waits for a task to finish after it has been Start()'ed
	Wait() error
	// Kill forcefully terminates the task
	Kill() error
	// Run starts the task in the foreground and waits for it to finish
	Run() error
}

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

// FanOut runs multiple process-like tasks with a maximum number of concurrent tasks, like the shell & operator
func FanOut(parallelCount int, cmds ...Commander) error {
	log.Println(parallelCount, len(cmds))
	cmdChan := make(chan Commander)
	errChan := make(chan error)
	sem := make(chan struct{})
	go func() {
		for _, cmd := range cmds {
			cmdChan <- cmd
		}
		close(cmdChan)
		log.Println("all commands pushed")
	}()
	for ix := 0; ix < parallelCount; ix++ {
		go func() {
			log.Println("fanout started")
			defer func() {
				sem <- struct{}{}
				log.Println("sem++")
				log.Println("fanout finished")
			}()
			for cmd := range cmdChan {
				errChan <- cmd.Run()
				log.Println("Wrote err")
			}
		}()
	}
	go func() {
		for ix := 0; ix < parallelCount; ix++ {
			_ = <-sem
			log.Println("sem--")
		}
		log.Println("all fanouts finished")
		close(errChan)
	}()
	errs := make([]error, 0, len(cmds))
	for err := range errChan {
		log.Println("Read err")
		if err != nil {
			errs = append(errs, err)
		}
	}
	log.Println("all errors recorded")
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// A Cmd is a wrapper for building os/exec.Cmd's
type Cmd struct {
	*exec.Cmd
	// HandleStdout is the function, if any, to feed stdout to
	HandleStdout PipeProcessor
	// HandleStdoutErr is a channel that will be send the error returned by HandleStdout
	HandleStdoutErr chan error
	// Closers is a set of things that should be closed after the Cmd finishes
	Closers []io.Closer
}

var (
	_ = Commander(&Cmd{})
)

// Command returns a new command
func Command(ctx context.Context, cmd ...string) *Cmd {
	return &Cmd{Cmd: exec.CommandContext(ctx, cmd[0], cmd[1:]...), Closers: make([]io.Closer, 0)}
}

// ForwardAll forwards stdin/out/err to/from the current process from/to this Cmd
func (c *Cmd) ForwardAll() *Cmd {
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// ForwardOutErr forwards stdout/err to the current process from this Cmd
func (c *Cmd) ForwardOutErr() *Cmd {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// ForwardErr forward stderr to the current process from this Cmd
func (c *Cmd) ForwardErr() *Cmd {
	c.Stderr = os.Stderr
	return c
}

// ProcessOut sets a function to feed the stdout of this Cmd into
func (c *Cmd) ProcessOut(handler PipeProcessor) *Cmd {
	c.HandleStdout = handler
	c.HandleStdoutErr = make(chan error)
	return c
}

// StringIn sets a literal string to be provided as stdin to this Cmd
func (c *Cmd) StringIn(in string) *Cmd {
	c.Stdin = strings.NewReader(in)
	return c
}

// FileIn sets the path of a file whose contents are to be to redirected to this Cmd's stdin
func (c *Cmd) FileIn(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	c.Cmd.Stdin = f
	c.Closers = append(c.Closers, f)
	return nil
}

// FileOut sets the path of a file to redirect this Cmd's stdout to
func (c *Cmd) FileOut(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	c.Cmd.Stdout = f
	c.Closers = append(c.Closers, f)
	return nil
}

// FileOut sets the path of a file to redirect this Cmd's stderr to
func (c *Cmd) FileErr(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	c.Cmd.Stderr = f
	c.Closers = append(c.Closers, f)
	return nil
}

// WithParentEnv copies the current process's environment variables to this Cmd
func (c *Cmd) WithParentEnv() *Cmd {
	c.Env = make([]string, len(os.Environ()))
	copy(c.Env, os.Environ())
	return c
}

// WithEnv sets one or more environment variables for this Cmd. Note that if you do not call WithParentEnv() first, the current process's variables will not be passed.
func (c *Cmd) WithEnv(env map[string]string) *Cmd {
	envIndex := make(map[string]int, len(c.Cmd.Env))
	for ix, envLine := range c.Cmd.Env {
		split := strings.SplitN(envLine, "=", 2)
		envIndex[split[0]] = ix
	}
	for k, v := range env {
		line := fmt.Sprintf("%s=%s", k, v)
		ix, exists := envIndex[k]
		if exists {
			c.Cmd.Env[ix] = line
		} else {
			c.Cmd.Env = append(c.Cmd.Env, line)
		}
	}
	return c
}

func (c *Cmd) startStdoutProcessor() error {
	if c.HandleStdout != nil {
		stdout, err := c.Cmd.StdoutPipe()
		if err != nil {
			return err
		}
		go func() {
			defer func() {
				r := recover()
				if r == nil {
					return
				}
				err, ok := r.(error)
				if ok {
					c.HandleStdoutErr <- err
				} else {
					c.HandleStdoutErr <- fmt.Errorf("panicked: %#v", r)
				}
				close(c.HandleStdoutErr)
			}()
			c.HandleStdoutErr <- c.HandleStdout(stdout)
		}()
	}
	return nil
}

// Run implements Commander
func (c *Cmd) Run() error {
	defer func() {
		for _, closer := range c.Closers {
			closer.Close()
		}
	}()
	err := c.startStdoutProcessor()
	if err != nil {
		return err
	}
	log.Println(c.Path, c.Args)
	err = c.Cmd.Run()
	log.Println("exited", c.Path, c.Args)
	if err != nil {
		return err
	}
	if c.HandleStdoutErr == nil {
		return nil
	}
	err = <-c.HandleStdoutErr
	if err != nil {
		return err
	}
	return nil
}

// Start implements Commander
func (c *Cmd) Start() error {
	err := c.startStdoutProcessor()
	if err != nil {
		return err
	}
	log.Println(c.Path, c.Args, "&")
	return c.Cmd.Start()
}

// Wait implements Commander
func (c *Cmd) Wait() error {
	defer func() {
		for _, closer := range c.Closers {
			closer.Close()
		}
	}()

	log.Println("waiting", c.Path, c.Args)
	err := c.Cmd.Wait()
	log.Println("exited", c.Path, c.Args)
	if err != nil {
		return err
	}
	if c.HandleStdoutErr == nil {
		return nil
	}
	err = <-c.HandleStdoutErr
	if err != nil {
		return err
	}
	return nil
}

// Kill implements Commander
func (c *Cmd) Kill() error {
	return c.Cmd.Process.Kill()
}

// A Pipeline is two or more processes, run in parallel, which feed the stdout of each process to the next process's stdin like a standard shell pipe
type Pipeline struct {
	// Cmds are the commands to run, in order
	Cmds []*Cmd
	// InPipes are the read side of the pipes
	InPipes []*os.File
	// OutPipes are the write side of the pipes
	OutPipes []*os.File
}

var (
	_ = Commander(&Pipeline{})
)

// NewPipeline creates a new pipeline
func NewPipeline(cmd ...*Cmd) (*Pipeline, error) {
	if len(cmd) < 2 {
		return nil, fmt.Errorf("Need at least two commands for a pipeline")
	}
	prevCmd := cmd[0]
	inPipes := make([]*os.File, 0, len(cmd)-1)
	outPipes := make([]*os.File, 0, len(cmd)-1)
	for _, nextCmd := range cmd[1:] {
		// TODO: Does this need to get cleaned up somehow?
		reader, writer, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		inPipes = append(inPipes, reader)
		outPipes = append(outPipes, writer)
		nextCmd.Stdin = reader
		prevCmd.Stdout = writer
	}
	return &Pipeline{Cmds: cmd, InPipes: inPipes, OutPipes: outPipes}, nil
}

// ForwardErr forwrds the stderr of all commands in the pipeline to the current process
func (p *Pipeline) ForwardErr() *Pipeline {
	for _, cmd := range p.Cmds {
		cmd.ForwardErr()
	}
	return p
}

// Run implements Commander
func (p *Pipeline) Run() error {
	err := p.Start()
	if err != nil {
		return err
	}
	err = p.Wait()
	log.Println("pipeline finished")
	if err != nil {
		return err
	}
	return nil
}

// Start implements Commander
func (p *Pipeline) Start() error {
	for ix, cmd := range p.Cmds {
		err := cmd.Start()
		if err != nil {
			for ix2 := 0; ix2 < ix; ix2++ {
				p.Cmds[ix2].Process.Kill()
			}
			return err
		}
	}
	return nil
}

type ixerr struct {
	ix  int
	err error
}

// Wait implements Commander
func (p *Pipeline) Wait() error {
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
		go func(ix int, cmd *Cmd) {
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
func (p *Pipeline) Kill() error {
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

// An And runs a sequence of tasks, stopping at the first failure, like the shell && operator
type And struct {
	// Cmds are the tasks to run
	Cmds   []*Cmd
	events chan sequenceEvent
	result chan error
}

var (
	_ = Commander(&And{})
)

// Run implements Commander
func (a *And) Run() error {
	for _, cmd := range a.Cmds {
		err := cmd.Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *And) Start() error {
	a.events = make(chan sequenceEvent)
	a.result = make(chan error)
	go func() {
		defer func() {
			r := recover()
			if r != nil {
				// TODO: Send result
			}
		}()
		for ix, cmd := range a.Cmds {
			go func(ix int, cmd *Cmd) {
				defer func() {
					r := recover()
					if r != nil {
						// TODO: send event
					}
				}()
				err := cmd.Run()
				if err != nil {
					a.events <- sequenceEvent{err: &ixerr{ix: ix, err: err}}
					return
				}
				a.events <- sequenceEvent{next: &ix}
			}(ix, cmd)
			event := <-a.events
			if event.kill != nil {
				// TODO: kill current process
				return
			}
			if event.err != nil {
				a.result <- event.err.err
				return
			}
			if event.next != nil {
				continue
			}
		}
		a.result <- nil
	}()
	// TODO: Should at least the first process be started synchronously?
	return nil
}

func (a *And) Kill() error {
	for _, cmd := range a.Cmds {
		cmd.Kill()
	}
	// TODO: Should this return a MultiProcessError?
	return nil
}

func (a *And) Wait() error {
	return <-a.result
}

// An Or runs a sequence of tasks, stopping at the first success, like the shell || operator
type Or struct {
	// Cmds are the tasks to run
	Cmds   []*Cmd
	events chan sequenceEvent
	result chan error
}

var (
	_ = Commander(&Or{})
)

// Run implements Commander
func (o *Or) Run() error {
	for _, cmd := range o.Cmds {
		err := cmd.Run()
		if err == nil {
			break
		}
	}
	return nil
}

func (o *Or) Start() error {
	o.events = make(chan sequenceEvent)
	o.result = make(chan error)
	go func() {
		defer func() {
			r := recover()
			if r != nil {
				// TODO: Send result
			}
		}()
		for ix, cmd := range o.Cmds {
			go func(ix int, cmd *Cmd) {
				defer func() {
					r := recover()
					if r != nil {
						// TODO: send event
					}
				}()
				err := cmd.Run()
				if err != nil {
					o.events <- sequenceEvent{err: &ixerr{ix: ix, err: err}}
					return
				}
				o.events <- sequenceEvent{next: &ix}
			}(ix, cmd)
			event := <-o.events
			if event.kill != nil {
				// TODO: kill current process
				return
			}
			if event.err != nil {
				continue
			}
			if event.next != nil {
				return
			}
		}
		o.result <- nil
	}()
	return nil
}

func (o *Or) Kill() error {
	// TODO: Should these be killed in reverse?
	for _, cmd := range o.Cmds {
		cmd.Kill()
	}
	// TODO: Should this return a MultiProcessError?
	return nil
}

func (o *Or) Wait() error {
	return <-o.result
}

// A Then runs a sequence of tasks, ignoring, but recording, any fialures, like the shell ; operator
type Then struct {
	// Cmds are the tasks to run
	Cmds   []*Cmd
	events chan sequenceEvent
	result chan error
}

var (
	_ = Commander(&Then{})
)

// Run implements Commander
func (t *Then) Run() error {
	errs := make([]error, 0, len(t.Cmds))
	for _, cmd := range t.Cmds {
		err := cmd.Run()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

func (t *Then) Start() error {
	t.events = make(chan sequenceEvent)
	t.result = make(chan error)
	go func() {
		defer func() {
			r := recover()
			if r != nil {
				// TODO: Send result
			}
		}()
		for ix, cmd := range t.Cmds {
			go func(ix int, cmd *Cmd) {
				defer func() {
					r := recover()
					if r != nil {
						// TODO: send event
					}
				}()
				err := cmd.Run()
				if err != nil {
					t.events <- sequenceEvent{err: &ixerr{ix: ix, err: err}}
					return
				}
				t.events <- sequenceEvent{next: &ix}
			}(ix, cmd)
			event := <-t.events
			if event.kill != nil {
				// TODO: kill current process
				return
			}
			if event.err != nil {
				// TODO: Should this err be recorded?
				continue
			}
			if event.next != nil {
				continue
			}
		}
		t.result <- nil
	}()
	return nil
}

func (t *Then) Kill() error {
	// TODO: Should these be killed in reverse?
	for _, cmd := range t.Cmds {
		cmd.Kill()
	}
	// TODO: Should this return a MultiProcessError?
	return nil
}

func (t *Then) Wait() error {
	return <-t.result
}
