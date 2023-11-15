package gosh

import (
	"io"
)

type sequenceEvent struct {
	kill *struct{}
	err  *ixerr
	next *int
}

// A SequenceGate indicates to a Sequence what to do when a command finishes, and has a chance to modify the final error
type SequenceGate func(s *SequenceCmd, ix int, err error, killed bool) (continu bool, finalErr error)

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

// Sequence creates a new sequence of commands with a function to determine when to stop
func Sequence(gate SequenceGate, cmds ...Commander) *SequenceCmd {
	return &SequenceCmd{Gate: gate, Cmds: cmds, CmdErrors: make([]error, 0, len(cmds))}
}

// Run implements Commander
func (s *SequenceCmd) Run() error {
	if s.BuilderError != nil {
		return s.BuilderError
	}

	var err error
	var continu bool
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
		continu, err = s.Gate(s, ix, err, false)
		if !continu {
			break
		}
	}
	return err
}

// Start implements Commander
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
		var continu bool
	loop:
		for ix, cmd := range s.Cmds {
			select {
			case _ = <-s.kill:
				err = ErrKilled
				_, err = s.Gate(s, ix, err, true)
				break loop
			default:
				err = cmd.Run()
				if err != nil {
					s.CmdErrors = append(s.CmdErrors, err)
				}
				continu, err = s.Gate(s, ix, err, false)
				if !continu {
					break loop
				}
			}
		}
		s.result <- err
	}()
	return nil
}

// Kill implements Commander
func (s *SequenceCmd) Kill() error {
	s.kill <- struct{}{}
	return nil
}

// Wait implements Commander
func (s *SequenceCmd) Wait() (err error) {
	defer doDeferredAfter(&err, s.deferredAfter)
	err = <-s.result
	return
}

// SetStdin implements Pipelineable
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

// SetStdout implements Pipelineable
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

// SetStderr implements Pipelineable
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

// DeferBefore implements Pipelineable
func (s *SequenceCmd) DeferBefore(f func() error) {
	s.deferredBefore = append(s.deferredBefore, f)
}

// DeferAfter implements Pipelineable
func (s *SequenceCmd) DeferAfter(f func() error) {
	s.deferredAfter = append(s.deferredAfter, f)
}

// WithStreams applies a set of StreamSetters to this SequenceCmd
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

// And runs a sequence of tasks, stopping at the first failure, like the shell && operato
func And(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continu bool, finalError error) {
			if err != nil {
				return false, err
			}
			return true, err
		},
		cmds...,
	)
}

// Or runs a sequence of tasks, stopping at the first success, like the shell || operator. If any command succeeds, the OrCmd succeeds. If all commands fail, the OrCmd fails with the final error.
func Or(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continu bool, finalError error) {
			if ix == len(s.Cmds)-1 {
				if err == nil {
					return true, nil
				}
				errs := make([]error, 0, len(s.CmdErrors))
				for _, cmdErr := range s.CmdErrors {
					errs = append(errs, cmdErr)
				}
				return true, &MultiProcessError{Errors: errs}
			}
			if err != nil {
				return true, err
			}
			return false, err
		},
		cmds...,
	)
}

// Then runs a sequence of tasks, ignoring, but recording, any failures, like the shell ; operator. If the last command succeds, then the Then succeeds, otherwise, it returns the error of the last command.
func Then(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continu bool, finalError error) {
			return true, err
		},
		cmds...,
	)
}

// All executes a all of a sequence of tasks, ignoring failures, but reports failure if any commands failed
func All(cmds ...Commander) *SequenceCmd {
	return Sequence(
		func(s *SequenceCmd, ix int, err error, killed bool) (continu bool, finalError error) {
			if ix == len(cmds)-1 && len(s.CmdErrors) != 0 && !killed {
				return true, &MultiProcessError{Errors: s.CmdErrors}
			}
			return true, err
		},
		cmds...,
	)
}
