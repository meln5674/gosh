package gosh

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"
)

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

// A PipeSource is a function which can produce input for a command
type PipeSource func(io.Writer) error

// A PipeSink is a function which can process the output (including standard error) of a command
type PipeSink func(io.Reader) error

// A StreamSetter modifies a command's streams
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

var (
	// ErrKilled is returned from Commander.Wait() if it is killed when no actual process is executing, otherwise, it will return whatever underlying error results from that process being killed
	ErrKilled = errors.New("Killed")

	// ErrNotStarted is returned from Commander.Wait() and Commander.Kill() if Start() was never called
	ErrNotStarted = errors.New("Not Started")

	// ErrAlreadyStarted is returned from Commander.Start() or Commander.Run() if either were already called
	ErrAlreadyStarted = errors.New("Already Started")
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

func forwardIn(p Pipelineable) error {
	return p.SetStdin(os.Stdin)
}

func forwardOut(p Pipelineable) error {
	return p.SetStdout(os.Stdout)
}

func forwardErr(p Pipelineable) error {
	return p.SetStderr(os.Stderr)
}

func forwardInOut(p Pipelineable) error {
	err := ForwardIn(p)
	if err != nil {
		return err
	}
	return ForwardOut(p)
}

func forwardInErr(p Pipelineable) error {
	err := ForwardIn(p)
	if err != nil {
		return err
	}
	return ForwardErr(p)
}

func forwardOutErr(p Pipelineable) error {
	err := ForwardOut(p)
	if err != nil {
		return err
	}
	return ForwardErr(p)
}

func forwardAll(p Pipelineable) error {
	err := ForwardIn(p)
	if err != nil {
		return err
	}
	return ForwardOutErr(p)
}

var (
	// ForwardIn sets a a command's stdin to the current process's stdin
	ForwardIn StreamSetter = forwardIn
	// ForwardOut sets a a command's stdout to the current process's stdout
	ForwardOut StreamSetter = forwardOut
	// ForwardErr sets a a command's stderr to the current process's stderr
	ForwardErr StreamSetter = forwardErr
	// ForwardInOut does both ForwardIn and ForwardOut
	ForwardInOut StreamSetter = forwardInOut
	// ForwardInErr does both ForwardIn and ForwardErr
	ForwardInErr StreamSetter = forwardInErr
	// ForwardOutErr does both ForwardOut and ForwardErr
	ForwardOutErr StreamSetter = forwardOutErr
	// ForwardAll does ForwardIn, ForwardOut, and ForwardErr
	ForwardAll StreamSetter = forwardAll
)

// SetStreams combines multiple StreamSetters into a single value. This is useful if you want to make utility StreamSetters that set multiple streams.
func SetStreams(setters ...StreamSetter) StreamSetter {
	return func(p Pipelineable) error {
		for _, setter := range setters {
			err := setter(p)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// ReaderIn sets a reader as Stdin, equivalent to calling SetStdin, but usable with WithStreams
func ReaderIn(r io.Reader) StreamSetter {
	return func(p Pipelineable) error {
		return p.SetStdin(r)
	}
}

// WriterOut sets a writer as Stdout, equivalent to calling SetStdout, but usable with WithStreams
func WriterOut(w io.Writer) StreamSetter {
	return func(p Pipelineable) error {
		return p.SetStdout(w)
	}
}

// WriterErr sets a writer as Stderr, equivalent to calling SetStderr, but usable with WithStreams
func WriterErr(w io.Writer) StreamSetter {
	return func(p Pipelineable) error {
		return p.SetStderr(w)
	}
}

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
