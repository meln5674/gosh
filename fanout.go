package gosh

import (
	"errors"

	"github.com/go-logr/logr"
)

// A FanOutCmd runs a set of commands in parallel, with some limit as to how many can run concurrently
type FanOutCmd struct {
	Cmds           []Commander
	MaxConcurrency int
	Log            logr.Logger
	sem            chan struct{}
	cmdChan        chan Commander
	errChan        chan error
	kill           chan struct{}
	mutex          chan struct{}
	BuilderError   error
}

var (
	_ = Commander(&FanOutCmd{})
)

// FanOut creates a new FanOutCmd that lets all commands run at the same time
func FanOut(cmds ...Commander) *FanOutCmd {
	return &FanOutCmd{Cmds: cmds, MaxConcurrency: len(cmds)}
}

// WithLog sets a log to use for this fanout
func (f *FanOutCmd) WithLog(log logr.Logger) *FanOutCmd {
	f.Log = log
	return f
}

func (f *FanOutCmd) log() logr.Logger {
	if f.Log.IsZero() {
		return GlobalLog
	}
	return f.Log
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

// Run implements Commander
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
	f.cmdChan = make(chan Commander)
	f.errChan = make(chan error)
	f.sem = make(chan struct{})
	f.kill = make(chan struct{}, 1)
	f.mutex = make(chan struct{}, 1)
	go func() {
		for _, cmd := range f.Cmds {
			f.cmdChan <- cmd
		}
		close(f.cmdChan)
		f.log().V(DebugLogLevel).Info("all commands pushed")
	}()
	for ix := 0; ix < f.MaxConcurrency; ix++ {
		go func() {
			f.log().V(DebugLogLevel).Info("fanout started")
			defer func() {
				f.sem <- struct{}{}
				f.log().V(DebugLogLevel).Info("sem++")
				f.log().V(DebugLogLevel).Info("fanout finished")
			}()
			killed := false
			for !killed {
				func() {
					var cmd Commander
					var ok bool
					var started bool

					f.sync(func() {
						select {
						case cmd, ok = <-f.cmdChan:
							if !ok {
								killed = true
								return
							}
							err := cmd.Start()
							if err != nil {
								f.log().V(DebugLogLevel).Info("Wrote err")
								f.errChan <- err
								return
							}
							started = true

						case _ = <-f.kill:
							f.log().V(DebugLogLevel).Info("Got kill signal, emptying cmdChan")
							for range f.cmdChan {
							}
							killed = true
							return
						}
					})
					if killed {
						return
					}
					if started {
						f.log().V(DebugLogLevel).Info("Wrote err")
						f.errChan <- cmd.Wait()
					}
				}()
			}
		}()
	}
	go func() {
		for ix := 0; ix < f.MaxConcurrency; ix++ {
			_ = <-f.sem
			f.log().V(DebugLogLevel).Info("sem--")
		}
		f.log().V(DebugLogLevel).Info("all fanouts finished")
		close(f.errChan)
	}()
	return nil
}

// Wait implements Commander
func (f *FanOutCmd) Wait() error {
	errs := make([]error, 0, len(f.Cmds))
	for err := range f.errChan {
		f.log().V(DebugLogLevel).Info("Read err")
		if err != nil {
			errs = append(errs, err)
		}
	}
	f.log().V(DebugLogLevel).Info("all errors recorded")
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// Kill implements Commander
func (f *FanOutCmd) Kill() error {
	errs := make([]error, 0, len(f.Cmds))
	f.sync(func() {
		close(f.kill)
		for range f.cmdChan {
		}
		for _, cmd := range f.Cmds {
			err := cmd.Kill()
			if err != nil && !errors.Is(err, ErrNotStarted) {
				errs = append(errs, err)
			}
		}
	})
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

func (f *FanOutCmd) sync(fn func()) {
	f.mutex <- struct{}{}
	defer func() { _ = <-f.mutex }()
	fn()
}
