package gosh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/meln5674/gosh"
	"io"
	"io/ioutil"
	"os"
	"strings"
)

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

var _ = error(&testError{})

var _ = Describe("MultiProcessError)", func() {
	When("multiple errors present", func() {
		err := gosh.MultiProcessError{Errors: []error{
			&testError{"foo"},
			&testError{"bar"},
			&testError{"baz"},
		}}
		It("should contain each errors' Error() as a substring", func() {
			msg := err.Error()
			Expect(msg).To(ContainSubstring(err.Errors[0].Error()))
			Expect(msg).To(ContainSubstring(err.Errors[1].Error()))
			Expect(msg).To(ContainSubstring(err.Errors[2].Error()))
		})
	})
})

func saveStd() (stdin, stdout, stderr *os.File) {
	stdin = os.Stdin
	stdout = os.Stdout
	stderr = os.Stderr
	return
}

func mockStd() (stdin, stdout, stderr *bytes.Buffer, start, close func()) {
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	stderrRead, stderrWrite, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	stdin = bytes.NewBufferString("")
	stdout = bytes.NewBufferString("")
	stderr = bytes.NewBufferString("")

	start = func() {
		go func() {
			if _, err := io.CopyN(stdinWrite, stdin, int64(stdin.Len())); err != nil {
				panic(err)
			}
			if err := stdinWrite.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				panic(err)
			}
		}()
		go io.Copy(stdout, stdoutRead)
		go io.Copy(stderr, stderrRead)
	}
	close = func() {
		stdinWrite.Close()
		stdoutWrite.Close()
		stderrWrite.Close()
	}
	os.Stdin = stdinRead
	os.Stdout = stdoutWrite
	os.Stderr = stderrWrite
	return
}

func restoreStd(stdin, stdout, stderr *os.File) {
	os.Stdin = stdin
	os.Stdout = stdout
	os.Stderr = stderr
}

var _ = Describe("Cmd", func() {
	var stdin, stdout, stderr *os.File
	var mockStdin, mockStdout, mockStderr *bytes.Buffer
	var startMocks, stopMocks func()
	pushMock := func() {
		stdin, stdout, stderr = saveStd()
		mockStdin, mockStdout, mockStderr, startMocks, stopMocks = mockStd()
		Expect(os.Stdin.Fd()).ToNot(Equal(uintptr(0)))
	}
	popMock := func() {
		restoreStd(stdin, stdout, stderr)
	}

	When("Forwarding in, out and err", func() {
		BeforeEach(pushMock)
		AfterEach(popMock)

		It("should forward them", func() {
			stdinString := "in"
			expectedStdout := stdinString
			expectedStderr := "err"
			mockStdin.WriteString(stdinString)
			startMocks()
			testSh := fmt.Sprintf("cat ; echo -n '%s' >&2", expectedStderr)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh).ForwardAll()
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			Expect(mockStdin.Len()).To(Equal(0))
			Expect(mockStderr.String()).To(Equal(expectedStderr))
			Expect(mockStdout.String()).To(Equal(expectedStdout))
		})
	})
	When("Forwarding out and err", func() {
		BeforeEach(pushMock)
		AfterEach(popMock)

		It("should forward them but not in", func() {
			stdinString := "in"
			expectedStdout := ""
			expectedStderr := "err"
			mockStdin.WriteString(stdinString)
			startMocks()
			testSh := fmt.Sprintf("cat ; echo -n '%s' >&2", expectedStderr)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh).ForwardOutErr()
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			Expect(mockStderr.String()).To(Equal(expectedStderr))
			Expect(mockStdout.String()).To(Equal(expectedStdout))
		})
	})
	When("Forwarding err", func() {
		BeforeEach(pushMock)
		AfterEach(popMock)

		It("should forward it but not out or err", func() {
			stdinString := "in"
			expectedStdout := ""
			expectedStderr := "err"
			mockStdin.WriteString(stdinString)
			startMocks()
			testSh := fmt.Sprintf("echo 'something' ; echo -n '%s' >&2", expectedStderr)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh).ForwardErr()
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			Expect(mockStderr.String()).To(Equal(expectedStderr))
			Expect(mockStdout.String()).To(Equal(expectedStdout))
		})
	})
	When("Processing out", func() {

		It("should forward them", func() {
			expectedStdout := "out"
			expectedProcessedStdout := expectedStdout + expectedStdout + expectedStdout
			startMocks()
			testSh := fmt.Sprintf("echo -n '%s'", expectedStdout)
			var processedOut string
			cmd := gosh.
				Command(context.TODO(), "bash", "-c", testSh).
				ProcessOut(func(r io.Reader) error {
					out, err := io.ReadAll(r)
					if err != nil {
						return err
					}
					processedOut = strings.Repeat(string(out), 3)
					return nil
				})
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			Expect(processedOut).To(Equal(expectedProcessedStdout))
		})
	})
	When("Using a string for in", func() {

		It("should forward them", func() {
			stdinString := "in"
			startMocks()
			testSh := fmt.Sprintf(`[ "$(cat)" == '%s' ]`, stdinString)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh).StringIn(stdinString)
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			Expect(mockStdin.Len()).To(Equal(0))
		})
	})
	When("Using a file for in", func() {

		It("should forward them", func() {
			stdinString := "in"
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			_, err = f.Write([]byte(stdinString))
			Expect(err).ToNot(HaveOccurred())
			startMocks()
			testSh := fmt.Sprintf(`[ "$(cat)" == '%s' ]`, stdinString)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh)
			Expect(cmd.FileIn(f.Name())).To(Succeed())
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
		})
	})
	When("Using a missing file for in", func() {

		It("should fail", func() {
			startMocks()
			testSh := fmt.Sprintf(`echo "This should fail"`)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh)
			Expect(cmd.FileIn("this file doesn't exist")).ToNot(Succeed())
			stopMocks()
		})
	})
	When("Using a file for out", func() {

		It("should forward them", func() {
			expectedStdout := "out"
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			startMocks()
			testSh := fmt.Sprintf("echo -n '%s'", expectedStdout)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh)
			Expect(cmd.FileOut(f.Name())).To(Succeed())
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal(expectedStdout))
		})
	})
	When("Using an unwritable file for out", func() {

		It("should fail", func() {
			expectedStdout := "out"
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())
			startMocks()
			testSh := fmt.Sprintf("echo -n '%s'", expectedStdout)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh)
			Expect(cmd.FileOut(f.Name())).ToNot(Succeed())
			stopMocks()
		})
	})
	When("Using a file for err", func() {

		It("should forward them", func() {
			expectedStderr := "err"
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			startMocks()
			testSh := fmt.Sprintf("echo -n '%s' >&2", expectedStderr)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh)
			Expect(cmd.FileErr(f.Name())).To(Succeed())
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal(expectedStderr))
		})
	})
	When("Using an unwritable file for err", func() {

		It("should fail", func() {
			expectedStderr := "err"
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())
			startMocks()
			testSh := fmt.Sprintf("echo -n '%s'", expectedStderr)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh)
			Expect(cmd.FileErr(f.Name())).ToNot(Succeed())
			stopMocks()
		})
	})

	When("overriding parent environment variables", func() {
		It("Should set specified variables and include all others", func() {
			currentHome, err := os.UserHomeDir()
			Expect(err).ToNot(HaveOccurred())
			dummyHostname := "foo"
			dummyVar := "BAR"
			dummyVarValue := "baz"
			testSh := fmt.Sprintf(`[ "${HOME}" == '%s' ] && [ "${HOSTNAME}" == '%s' ] && [ "${%s}" == '%s' ]`, currentHome, dummyHostname, dummyVar, dummyVarValue)
			cmd := gosh.Command(context.TODO(), "bash", "-c", testSh).WithParentEnvAnd(map[string]string{"HOSTNAME": dummyHostname, dummyVar: dummyVarValue})
			Expect(cmd.Run()).To(Succeed())
		})
	})
})
