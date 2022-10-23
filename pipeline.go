package gosh

import (
	"fmt"
	"io"
	"os"
)

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

// DeferBefore implements Pipelineable
func (p *PipelineCmd) DeferBefore(f func() error) {
	p.Cmds[0].DeferBefore(f)
}

// DeferAfter implements Pipelineable
func (p *PipelineCmd) DeferAfter(f func() error) {
	p.Cmds[len(p.Cmds)-1].DeferAfter(f)
}

// WithStreams applies a set of StreamSetters to this pipeline. Input goes to the first command, output and error come from the last command
func (p *PipelineCmd) WithStreams(fs ...StreamSetter) *PipelineCmd {
	if p.BuilderError != nil {
		return p
	}
	for _, f := range fs {
		f(p)
	}
	return p
}

// SetStdin implements Pipelineable by setting stdin for the first command in the pipeline
func (p *PipelineCmd) SetStdin(stdin io.Reader) error {
	if p.BuilderError != nil {
		return nil
	}
	return p.Cmds[0].SetStdin(stdin)
}

// SetStdout implements Pipelineable by setting stdout for the last command in the pipeline
func (p *PipelineCmd) SetStdout(stdout io.Writer) error {
	if p.BuilderError != nil {
		return nil
	}
	return p.Cmds[len(p.Cmds)-1].SetStdout(stdout)
}

// SetStderr implements Pipelineable by setting stderr for all commands in the pipeline
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
		for range p.Cmds {
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
