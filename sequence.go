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
				err = ErrKilled
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
