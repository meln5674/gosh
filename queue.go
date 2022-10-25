package gosh

import (
	"errors"
	"k8s.io/klog/v2"
)

// A QueueCmd is like a FanOut, but instead of requiring all commands to be known upfront, it reads commands from a channel until it is closed
type QueueCmd struct {
	Cmds             chan Commander
	MaxConcurrency   int
	ResultBufferSize int
	sem              chan struct{}
	errChan          chan error
	kill             chan struct{}
	killErrs         chan error
	mutex            chan struct{}
	BuilderError     error
}

var (
	_ = Commander(&QueueCmd{})
)

func (q *QueueCmd) sync(f func()) {
	q.mutex <- struct{}{}
	defer func() { _ = <-q.mutex }()
	f()
}

// Queue creates a new QueueCmd that lets all commands run at the same time
func Queue(cmds chan Commander) *QueueCmd {
	return &QueueCmd{Cmds: cmds}
}

// WithMaxConcurrency sets the maximum number of commans that can run at the same time
func (q *QueueCmd) WithMaxConcurrency(max int) *QueueCmd {
	if max < 1 {
		q.BuilderError = errors.New("Max Concurrency must be Positive")
		return q
	}
	q.MaxConcurrency = max
	return q
}

// WithResultBufferSize sets the size of the result (error) channel buffer.
// This is ignored if MaxConcurrency is not set.
// If MaxConcurrency is set, the buffer size defaults to the same value.
// This means that sending a new command will block until a goroutine becomes available to
// run that command.
// If you potentially need to close the command channel (for example, an error is detect and you need
// to exit early), you can set this to ensure that you can send at least this many commands
// before blocking, preventing a deadlock, as well as batching up new commands to prevent a panic
// from sending on that closed channel.
// Make sure to also buffer your command channel in that case.
func (q *QueueCmd) WithResultBufferSize(size int) *QueueCmd {
	q.ResultBufferSize = size
	return q
}

// Run implements Commander
func (q *QueueCmd) Run() error {
	if q.BuilderError != nil {
		return q.BuilderError
	}
	err := q.Start()
	if err != nil {
		return err
	}
	err = q.Wait()
	if err != nil {
		return err
	}
	return nil
}

// Start implements Commander
func (q *QueueCmd) Start() error {
	if q.BuilderError != nil {
		return q.BuilderError
	}
	// If we are bounded, then a finished process will be waiting to report its error to this channel,
	// preventing the goroutine from continuing to the next command, so we must be able to buffer
	// all such errors in case the user is not calling Wait() in order to actually achieve the desired
	// level of concurrency
	// If we are unbounded, then we will start as many goroutines as commands, so there is no issue with an unbuffered channel
	if q.MaxConcurrency < 1 {
		q.errChan = make(chan error)
	} else if q.ResultBufferSize < 1 {
		q.errChan = make(chan error, q.MaxConcurrency)
	} else {
		q.errChan = make(chan error, q.ResultBufferSize)
	}
	q.sem = make(chan struct{})
	q.kill = make(chan struct{}, 1)
	q.killErrs = make(chan error)
	q.mutex = make(chan struct{}, 1)
	inFlight := make(map[Commander]struct{}, 0)
	killed := false
	done := false
	if q.MaxConcurrency < 1 {
		// If there is no cap, we start a goroutine which iterates over the command channel,
		// and starts goroutines for each command it gets
		// This goroutine uses a semaphore to count how many commands start. Once the channel closes,
		// it counts to make sure that many command goroutines finish, successfully or otherwise
		go func() {
			cmdsStarted := 0
			for !killed && !done {
				var cmd Commander
				var ok bool
				q.sync(func() {
					select {
					case cmd, ok = <-q.Cmds:
						if !ok {
							done = true
							return
						}
						klog.V(11).Info("Wrote err")
					case _ = <-q.kill:
						killed = true
						return
					}
					if killed || done {
						return
					}
					go func(cmd Commander) {
						defer func() {
							q.sem <- struct{}{}
							klog.V(11).Info("sem++")
							klog.V(11).Info("fanout finished")
						}()
						defer q.sync(func() {
							delete(inFlight, cmd)
						})
						q.sync(func() {
							err := cmd.Start()
							if err != nil {
								q.errChan <- err
								return
							}
							cmdsStarted++
							// TODO: Do we need a second lock just for inFlight?
							inFlight[cmd] = struct{}{}
						})
						q.errChan <- cmd.Wait()
					}(cmd)
				})
			}
			for ix := 0; ix < cmdsStarted; ix++ {
				_ = <-q.sem
				klog.V(11).Info("sem--")
			}

			klog.V(11).Info("all fanouts finished")
			close(q.errChan)
		}()
	} else {
		// If we have bounded number of jobs,
		// we start the number of requested goroutines which each iterate over the channel,
		// And start a second goroutuine which uses the same semaphore to wait for each
		// to exit
		for ix := 0; ix < q.MaxConcurrency; ix++ {
			go func() {
				klog.V(11).Info("fanout started")
				defer func() {
					q.sem <- struct{}{}
					klog.V(11).Info("sem++")
					klog.V(11).Info("fanout finished")
				}()
				for !killed && !done {
					func() {
						var cmd Commander
						var ok bool

						q.sync(func() {
							select {
							case cmd, ok = <-q.Cmds:
								if !ok {
									done = true
									return
								}
								klog.V(11).Info("Wrote err")
							case _ = <-q.kill:
								killed = true
								return
							}
						})
						if killed || done {
							return
						}
						func() {
							defer q.sync(func() {
								delete(inFlight, cmd)
							})
							q.sync(func() {
								err := cmd.Start()
								if err != nil {
									q.errChan <- err
									return
								}
								inFlight[cmd] = struct{}{}
							})
							q.errChan <- cmd.Wait()
						}()
					}()
				}
			}()
		}
		go func() {
			for ix := 0; ix < q.MaxConcurrency; ix++ {
				_ = <-q.sem
				klog.V(11).Info("sem--")
			}
			klog.V(11).Info("all fanouts finished")
			close(q.errChan)
		}()
	}
	// In both cases, we start a final goroutine which waits for the kill channel to be closed,
	// indicating that either the user has requested a Kill(), or that Wait() has finished.
	// In the first case, it obtains the lock on the set of running commands, and kills them, and
	// records their errors to a separate channel.
	// If the second case, there should be no commands remaining, so the same operation is a no-op.
	go func() {
		for range q.kill {
		}
		// Doing the loop under the lock results in a deadlock,
		// but simply := will just copy the pointer.
		// We have to copy in the lock, and then iterate the copy outside of the lock
		var inFlightCopy []Commander
		q.sync(func() {
			inFlightCopy = make([]Commander, 0, len(inFlight))
			for cmd := range inFlight {
				inFlightCopy = append(inFlightCopy, cmd)
			}
		})
		for _, cmd := range inFlightCopy {
			q.killErrs <- cmd.Kill()
		}

		close(q.killErrs)
	}()

	return nil
}

// Wait implements Commander. Wait blocks until the command channel is closed.
func (q *QueueCmd) Wait() error {
	errs := make([]error, 0)
	for err := range q.errChan {
		klog.V(11).Info("Read err")
		if err != nil {
			errs = append(errs, err)
		}
	}
	q.sync(func() {
		if q.kill != nil {
			close(q.kill)
		}
	})
	klog.V(11).Info("all errors recorded")
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}

// Kill implements Commander. Kill blocks until the command channel is closed.
func (q *QueueCmd) Kill() error {
	for range q.Cmds {
	}
	q.sync(func() {
		close(q.kill)
	})
	errs := make([]error, 0)
	for err := range q.killErrs {
		if err != nil && !errors.Is(err, ErrNotStarted) {
			errs = append(errs, err)
		}
	}
	q.sync(func() {
		q.kill = nil
	})
	if len(errs) != 0 {
		return &MultiProcessError{Errors: errs}
	}
	return nil
}
