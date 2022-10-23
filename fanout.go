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
					for range cmdChan {
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
