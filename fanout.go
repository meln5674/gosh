package gosh

import (
	"errors"
	"k8s.io/klog/v2"
)

// A FanOutCmd runs a set of commands in parallel, with some limit as to how many can run concurrently
type FanOutCmd struct {
	Cmds           []Commander
	MaxConcurrency int
	sem            chan struct{}
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
	cmdChan := make(chan Commander)
	f.errChan = make(chan error)
	f.sem = make(chan struct{})
	f.kill = make(chan struct{}, 1)
	f.mutex = make(chan struct{}, 1)
	go func() {
		for _, cmd := range f.Cmds {
			cmdChan <- cmd
		}
		close(cmdChan)
		klog.V(11).Info("all commands pushed")
	}()
	for ix := 0; ix < f.MaxConcurrency; ix++ {
		go func() {
			klog.V(11).Info("fanout started")
			defer func() {
				f.sem <- struct{}{}
				klog.V(11).Info("sem++")
				klog.V(11).Info("fanout finished")
			}()
			killed := false
			for !killed {
				func() {
					var cmd Commander
					var ok bool

					func() {
						f.mutex <- struct{}{}
						defer func() { _ = <-f.mutex }()
						select {
						case cmd, ok = <-cmdChan:
							if !ok {
								killed = true
								return
							}
							klog.V(11).Info("Wrote err")
						case _ = <-f.kill:
							klog.V(11).Info("Got kill signal, emptying cmdChan")
							for range cmdChan {
							}
							killed = true
							return
						}
					}()
					if killed {
						return
					}
					f.errChan <- cmd.Run()
				}()
			}
		}()
	}
	go func() {
		for ix := 0; ix < f.MaxConcurrency; ix++ {
			_ = <-f.sem
			klog.V(11).Info("sem--")
		}
		klog.V(11).Info("all fanouts finished")
		close(f.errChan)
	}()
	return nil
}

// Wait implements Commander
func (f *FanOutCmd) Wait() error {
	errs := make([]error, 0, len(f.Cmds))
	for err := range f.errChan {
		klog.V(11).Info("Read err")
		if err != nil {
			errs = append(errs, err)
		}
	}
	klog.V(11).Info("all errors recorded")
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// Kill implements Commander
func (f *FanOutCmd) Kill() error {
	f.kill <- struct{}{}
	errs := make([]error, 0, len(f.Cmds))
	func() {
		f.mutex <- struct{}{}
		defer func() { _ = <-f.mutex }()
		for _, cmd := range f.Cmds {
			err := cmd.Kill()
			if err != nil && !errors.Is(err, ErrNotStarted) {
				errs = append(errs, err)
			}
		}
	}()
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}
