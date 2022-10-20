package gosh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"bytes"
	"context"
	"fmt"
	"github.com/meln5674/gosh"
	"io"
	"os"
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
			if err := stdinWrite.Close(); err != nil {
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
			fmt.Fprintln(stdout, "Command terminated")
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
			fmt.Fprintln(stdout, "Command terminated")
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
			fmt.Fprintln(stdout, "Command terminated")
			Expect(cmd.Run()).To(Succeed())
			stopMocks()
			Expect(mockStderr.String()).To(Equal(expectedStderr))
			Expect(mockStdout.String()).To(Equal(expectedStdout))
		})
	})

})
