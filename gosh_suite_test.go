package gosh_test

import (
	. "github.com/meln5674/gosh/pkg/gomega"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"
	"testing"

	"bytes"
	"fmt"
	"github.com/meln5674/gosh"
	"io"
	"os"
	"strings"
)

func TestGosh(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gosh Suite")
}

var _ = BeforeSuite(func() {
	klog.SetOutput(GinkgoWriter)
})

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

func waitBeforeStart(f func() gosh.Commander) {
	When("waiting before starting", func() {
		It("should return ErrorNotStarted", func() {
			c := f()
			Expect(c.Wait()).To(Equal(gosh.ErrNotStarted))
		})
	})
}

func killBeforeStart(f func() gosh.Commander) {
	When("waiting before starting", func() {
		It("should return ErrorNotStarted", func() {
			c := f()
			Expect(c.Kill()).To(Equal(gosh.ErrNotStarted))
		})
	})
}

func doubleKill(f func() gosh.Commander) {
	When("kill is called twice", func() {
		It("should return ErrorNotStarted", func() {
			c := f()
			Expect(c.Start()).To(Succeed())
			Expect(c.Kill()).To(Succeed())
			Expect(c.Kill()).To(Equal(gosh.ErrNotStarted))
		})
	})
}

var (
	stdin, stdout, stderr                  *os.File
	mockStdin, mockStdout, mockStderr      *bytes.Buffer
	startMocks, stopMockIn, stopMockOutErr func()
	ginkgoWriter                           GinkgoWriterInterface
	mockLock                               = make(chan struct{}, 1)
)

func mockStd() (stdin, stdout, stderr *bytes.Buffer, start, closeIn, closeOutErr func()) {
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
	stdinDone := make(chan struct{})
	stdinDoneACK := make(chan struct{})

	stdoutDoneACK := make(chan struct{})
	stderrDoneACK := make(chan struct{})
	inClosed := false
	outErrClosed := false

	start = func() {
		go func() {
			defer GinkgoRecover()
		loop:
			for {
				select {
				case _ = <-stdinDone:
					break loop
				default:
					io.Copy(stdinWrite, stdin)
				}
			}
			io.Copy(stdinWrite, stdin)
			stdinDoneACK <- struct{}{}
		}()
		go func() {
			defer GinkgoRecover()
			io.Copy(stdout, stdoutRead)
			stdoutDoneACK <- struct{}{}
		}()
		go func() {
			defer GinkgoRecover()
			io.Copy(stderr, stderrRead)
			stderrDoneACK <- struct{}{}
		}()
	}
	closeIn = func() {
		if inClosed {
			return
		}
		inClosed = true
		stdinDone <- struct{}{}
		_ = <-stdinDoneACK
		stdinWrite.Close()
	}
	closeOutErr = func() {
		if outErrClosed {
			return
		}
		outErrClosed = true
		stdoutWrite.Close()
		stderrWrite.Close()
		for ix := 0; ix < 2; ix++ {
			select {
			case _ = <-stdoutDoneACK:
				break
			case _ = <-stderrDoneACK:
				break
			}
		}
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

func pushMock() {
	mockLock <- struct{}{}
	stdin, stdout, stderr = saveStd()
	mockStdin, mockStdout, mockStderr, startMocks, stopMockIn, stopMockOutErr = mockStd()
	//ginkgoWriter = GinkgoWriter
	//GinkgoWriter = &MockGinkgoWriter{Writer: stdout}
	Expect(os.Stdin.Fd()).ToNot(Equal(uintptr(0)))

}

func popMock() {
	_ = <-mockLock
	restoreStd(stdin, stdout, stderr)
	stdin = nil
	stdout = nil
	stderr = nil
	mockStdin = nil
	mockStdout = nil
	mockStderr = nil
	startMocks = nil
	stopMockIn = nil
	stopMockOutErr = nil
	GinkgoWriter = ginkgoWriter
}

type MockGinkgoWriter struct {
	io.Writer
}

var _ = GinkgoWriterInterface(&MockGinkgoWriter{})

func (m *MockGinkgoWriter) Print(a ...interface{}) {
	fmt.Fprint(m, a...)
}
func (m *MockGinkgoWriter) Printf(format string, a ...interface{}) {
	fmt.Fprintf(m, format, a...)
}
func (m *MockGinkgoWriter) Println(a ...interface{}) {
	fmt.Fprintln(m, a...)
}

func (m *MockGinkgoWriter) TeeTo(writer io.Writer) {
	// TODO: Needed?
}
func (m *MockGinkgoWriter) ClearTeeWriters() {
	// TODO: Needed?
}

func useMocks() {
	BeforeEach(pushMock)
	AfterEach(popMock)
}

type genericTestArgs struct {
	cmd          gosh.Commander
	stdin        string
	stdout       string
	stderr       string
	ignoreStdin  bool
	ignoreStdout bool
	ignoreStderr bool
	err          error
	errOnStart   bool
	async        bool
}

func genericTest(args genericTestArgs) {
	func() {
		startMocks()
		defer stopMockOutErr()
		if !args.ignoreStdin {
			_, err := mockStdin.WriteString(args.stdin)
			Expect(err).ToNot(HaveOccurred(), "Failed to write mock standard input")
		}
		stopMockIn()
		if args.async {
			if args.err == nil {
				Expect(args.cmd.Start()).To(Succeed(), "Command did not start")
				Expect(args.cmd.Wait()).To(Succeed(), "Command did not succeed")
			} else if args.errOnStart {
				Expect(args.cmd.Start()).To(MatchMultiProcessErrorType(args.err), "Did not fail in expected way")
			} else {
				Expect(args.cmd.Start()).To(Succeed(), "Command did not start")
				Expect(args.cmd.Wait()).To(MatchMultiProcessErrorType(args.err), "Did not fail in expected way")
			}
		} else if args.err == nil {
			Expect(args.cmd.Run()).To(Succeed(), "Command did not succeed")
		} else {
			Expect(args.cmd.Run()).To(MatchMultiProcessErrorType(args.err), "Did not fail in expected way")
		}
	}()
	if !args.ignoreStdin {
		Expect(mockStdin.String()).To(HaveLen(0), "Standard input was not consumed")
	}
	if !args.ignoreStdout {
		Expect(mockStdout.String()).To(Equal(args.stdout), "Standard output did not match")
	}
	if !args.ignoreStderr {
		Expect(mockStderr.String()).To(Equal(args.stderr), "Standard error did not match")
	}
}

func quoteShell(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", `'\''`))
}

func checkStdin(s string) string {
	return fmt.Sprintf(`[ "$(cat)" == %s ]`, quoteShell(s))
}

func checkStdinLine(s string) string {
	return fmt.Sprintf(`read && [ "${REPLY}" == %s ]`, quoteShell(s))
}

func checkStdinChar(c rune) string {
	return fmt.Sprintf(`[ "$(go run util/readchar/main.go)" == %s ]`, quoteShell(string(c)))
}

func printStdout(s string) string {
	return fmt.Sprintf(`echo -n %s`, quoteShell(s))
}

func printStderr(s string) string {
	return fmt.Sprintf(`echo -n %s >&2`, quoteShell(s))
}

func inOutPassthrough() string {
	return "cat"
}

func fail() string {
	return "exit 1"
}

func allOf(s ...string) string {
	return strings.Join(s, " && ")
}
