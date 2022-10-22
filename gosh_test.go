package gosh_test

import (
	. "github.com/meln5674/gosh/pkg/gomega"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"bytes"
	"errors"
	"flag"
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

func waitBeforeStart(f func() gosh.Commander) {
	When("waiting before starting", func() {
		It("should return ErrorNotStarted", func() {
			c := f()
			Expect(c.Wait()).To(Equal(gosh.ErrorNotStarted))
		})
	})
}

func killBeforeStart(f func() gosh.Commander) {
	When("waiting before starting", func() {
		It("should return ErrorNotStarted", func() {
			c := f()
			Expect(c.Kill()).To(Equal(gosh.ErrorNotStarted))
		})
	})
}

func doubleKill(f func() gosh.Commander) {
	When("kill is called twice", func() {
		It("should return ErrorNotStarted", func() {
			c := f()
			Expect(c.Start()).To(Succeed())
			Expect(c.Kill()).To(Succeed())
			Expect(c.Kill()).To(Equal(gosh.ErrorNotStarted))
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
		}()
		go func() {
			defer GinkgoRecover()
			io.Copy(stderr, stderrRead)
		}()
	}
	closeIn = func() {
		stdinDone <- struct{}{}
		_ = <-stdinDoneACK
		stdinWrite.Close()
	}
	closeOutErr = func() {
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

func genericSyncTest(cmd gosh.Commander, expectedStdin, expectedStdout, expectedStderr string) {
	func() {
		startMocks()
		defer stopMockOutErr()
		_, err := mockStdin.WriteString(expectedStdin)
		stopMockIn()
		Expect(err).ToNot(HaveOccurred(), "Failed to write mock standard input")
		Expect(cmd.Run()).To(Succeed(), "Command did not succeed")
	}()
	if len(expectedStdin) != 0 {
		Expect(mockStdin.String()).To(HaveLen(0), "Standard input was not consumed")
	}
	Expect(mockStdout.String()).To(Equal(expectedStdout), "Standard output did not match")
	Expect(mockStderr.String()).To(Equal(expectedStderr), "Standard error did not match")
}

func genericAsyncTest(cmd gosh.Commander, expectedStdin, expectedStdout, expectedStderr string) {
	func() {
		startMocks()
		defer stopMockOutErr()
		_, err := mockStdin.WriteString(expectedStdin)
		stopMockIn()
		Expect(err).ToNot(HaveOccurred(), "Failed to write mock standard input")
		Expect(cmd.Start()).To(Succeed(), "Command did not start")
		Expect(cmd.Wait()).To(Succeed(), "Command did not succeed")
	}()
	if len(expectedStdin) != 0 {
		Expect(mockStdin.String()).To(HaveLen(0), "Standard input was not consumed")
	}
	Expect(mockStdout.String()).To(Equal(expectedStdout), "Standard output did not match")
	Expect(mockStderr.String()).To(Equal(expectedStderr), "Standard error did not match")
}

func genericSyncFailTest(cmd gosh.Commander, expectedErr error) {
	startMocks()
	stopMockIn()
	defer stopMockOutErr()
	err := cmd.Run()
	Expect(err).To(HaveOccurred(), "Command did not fail")
	Expect(err).To(MatchMultiProcessError(expectedErr), "Failed differently than expected")
}

func genericAsyncFailTest(cmd gosh.Commander, expectedErr error) {
	startMocks()
	stopMockIn()
	defer stopMockOutErr()
	Expect(cmd.Start()).To(Succeed(), "Command did not start")
	err := cmd.Wait()
	Expect(err).To(HaveOccurred(), "Command did not fail")
	Expect(err).To(MatchMultiProcessError(expectedErr), "Failed differently than expected")
}

func genericAsyncFailStartTest(cmd gosh.Commander, expectedErr error) {
	startMocks()
	stopMockIn()
	defer stopMockOutErr()
	err := cmd.Start()
	Expect(err).To(HaveOccurred(), "Command did not fail to start")
	Expect(err).To(MatchMultiProcessError(expectedErr), "Failed differently than expected")
}

func quoteShell(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", `'\''`))
}

func checkStdin(s string) string {
	return fmt.Sprintf(`[ "$(cat)" == %s ]`, quoteShell(s))
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

func allOf(s ...string) string {
	return strings.Join(s, " && ")
}

var _ = Describe("Cmd", func() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "false")
	klog.SetOutput(GinkgoWriter)

	When("testing mocks", func() {
		useMocks()
		It("should work", func() {
			startMocks()
			klog.Info("This should go to the real stderr")
			stopMockIn()
			stopMockOutErr()
			Expect(mockStderr.String()).To(HaveLen(0))
		})
	})

	When("using not enough Args", func() {
		It("should fail", func() {
			Expect(gosh.Command().Run()).ToNot(Succeed())
			Expect(gosh.Command().Start()).ToNot(Succeed())
		})
	})

	When("Forwarding in, out and err", func() {
		useMocks()

		It("should forward them", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					inOutPassthrough(),
					printStderr("err"),
				)).WithStreams(gosh.ForwardAll),
				"in",
				"in",
				"err",
			)
		})
	})
	When("Forwarding in and err", func() {
		useMocks()

		It("should forward them", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("in"),
					printStdout("This shouldn't be seen"),
					printStderr("err"),
				)).WithStreams(gosh.ForwardInErr),
				"in",
				"",
				"err",
			)
		})
	})
	When("Forwarding in and out", func() {
		useMocks()

		It("should forward them", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("in"),
					printStdout("out"),
					printStderr("This shouldn't be seen"),
				)).WithStreams(gosh.ForwardInOut),
				"in",
				"out",
				"",
			)
		})
	})

	When("Forwarding out and err", func() {
		useMocks()

		It("should forward them but not in", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStdout("out"),
					printStderr("err"),
				)).WithStreams(gosh.ForwardOutErr),
				"This shouldn't be seen",
				"out",
				"err",
			)
		})
	})
	When("Forwarding in", func() {
		useMocks()

		It("should forward it but not out or err", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("in"),
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.ForwardIn),
				"in",
				"",
				"",
			)
		})
	})
	When("Forwarding out", func() {
		useMocks()

		It("should forward it but not out or err", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStdout("out"),
					printStderr("This shouldn't be seen"),
				)).WithStreams(gosh.ForwardOut),
				"This shouldn't be seen",
				"out",
				"",
			)
		})
	})
	When("Forwarding err", func() {
		useMocks()

		It("should forward it but not in or out", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStdout("This shouldn't be seen"),
					printStderr("err"),
				)).WithStreams(gosh.ForwardErr),
				"This shouldn't be seen",
				"",
				"err",
			)
		})
	})
	When("Processing in", func() {
		useMocks()

		It("should forward it", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("ininin"),
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					for ix := 0; ix < 3; ix++ {
						_, err := w.Write([]byte("in"))
						if err != nil {
							return err
						}
					}
					return nil
				})),
				"This shouldn't be seen",
				"",
				"",
			)
		})
	})
	When("Processing in fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericSyncFailTest(
				gosh.Shell(
					inOutPassthrough(),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					return err
				})),
				err,
			)
		})
	})
	When("Processing in fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericAsyncFailTest(
				gosh.Shell(
					inOutPassthrough(),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					return err
				})),
				err,
			)
		})
	})

	When("Processing out", func() {
		useMocks()

		It("should forward them", func() {
			var processedOut string
			genericSyncTest(
				gosh.Shell(allOf(
					printStdout("out"),
					printStderr("This shouldn't be seen"),
				)).WithStreams(gosh.FuncOut(gosh.SaveString(&processedOut))),
				"This shouldn't be seen",
				"",
				"",
			)
			Expect(processedOut).To(Equal("out"))
		})
	})
	When("Processing out fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericSyncFailTest(
				gosh.Shell(allOf(
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FuncOut(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})
	When("Processing out fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericAsyncFailTest(
				gosh.Shell(allOf(
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FuncOut(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})

	When("Processing err", func() {
		useMocks()

		It("should forward them", func() {
			var processedErr []byte
			genericSyncTest(
				gosh.Shell(allOf(
					printStderr("err"),
					printStdout("This shouldn't be seen"),
				)).WithStreams(gosh.FuncErr(gosh.SaveBytes(&processedErr))),
				"This shouldn't be seen",
				"",
				"",
			)
			Expect(string(processedErr)).To(Equal("err"))
		})
	})
	When("Processing err fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericSyncFailTest(
				gosh.Shell(allOf(
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FuncErr(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})
	When("Processing err fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericAsyncFailTest(
				gosh.Shell(allOf(
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FuncErr(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})
	When("Using a string for in", func() {
		useMocks()

		It("should send it", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("in"),
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.StringIn("in")),
				"",
				"",
				"",
			)
		})
	})
	When("Using a bytes for in", func() {
		useMocks()

		It("should send it", func() {
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("in"),
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.BytesIn([]byte("in"))),
				"",
				"",
				"",
			)
		})
	})
	When("Using a file for in", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			_, err = f.Write([]byte("in"))
			Expect(err).ToNot(HaveOccurred())
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin("in"),
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FileIn(f.Name())),
				"",
				"",
				"",
			)
		})
	})
	When("Using a missing file for in", func() {
		useMocks()

		It("should fail", func() {
			genericSyncFailTest(
				gosh.Shell(allOf(
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FileIn("This file doesn't exist")),
				os.ErrNotExist,
			)
		})
		It("should fail asynchronously", func() {
			genericAsyncFailStartTest(
				gosh.Shell(allOf(
					printStdout("This shouldn't be seen"),
					printStderr("Neither should this"),
				)).WithStreams(gosh.FileIn("This file doesn't exist")),
				os.ErrNotExist,
			)
		})
	})

	When("Using a file for out", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStdout("out"),
					printStderr("This shouldn't be seen"),
				)).WithStreams(gosh.FileOut(f.Name())),
				"",
				"",
				"",
			)
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal("out"))
		})
	})
	When("Using an unwritable file for out", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericSyncFailTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStdout("out"),
					printStderr("This shouldn't be seen"),
				)).WithStreams(gosh.FileOut(f.Name())),
				os.ErrPermission,
			)
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal(""))
		})
	})
	When("Using an unwritable file for out asynchronously", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericAsyncFailStartTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStdout("out"),
					printStderr("This shouldn't be seen"),
				)).WithStreams(gosh.FileOut(f.Name())),
				os.ErrPermission,
			)
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal(""))
		})
	})

	When("Using a file for err", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			genericSyncTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStderr("err"),
					printStdout("This shouldn't be seen"),
				)).WithStreams(gosh.FileErr(f.Name())),
				"",
				"",
				"",
			)
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal("err"))
		})
	})
	When("Using an unwritable file for err", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericSyncFailTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStderr("err"),
					printStdout("This shouldn't be seen"),
				)).WithStreams(gosh.FileErr(f.Name())),
				os.ErrPermission,
			)
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal(""))
		})
	})
	When("Using an unwritable file for err asynchronously", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericAsyncFailStartTest(
				gosh.Shell(allOf(
					checkStdin(""),
					printStderr("err"),
					printStdout("This shouldn't be seen"),
				)).WithStreams(gosh.FileErr(f.Name())),
				os.ErrPermission,
			)
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal(""))
		})
	})
	When("overriding parent environment variables", func() {
		It("Should set specified variables and include all others", func() {
			currentHome, err := os.UserHomeDir()
			Expect(err).ToNot(HaveOccurred())
			dummyUser := "foo"
			dummyVar := "BAR"
			dummyVarValue := "baz"
			testSh := fmt.Sprintf(`[ "${HOME}" == '%s' ] && [ "${USER}" == '%s' ] && [ "${%s}" == '%s' ]`, currentHome, dummyUser, dummyVar, dummyVarValue)
			cmd := gosh.Command("bash", "-c", testSh).WithParentEnvAnd(map[string]string{"USER": dummyUser, dummyVar: dummyVarValue})
			Expect(cmd.Run()).To(Succeed())
		})
	})

	When("using Start() and Wait()", func() {
		It("should be the same as using Run()", func() {
			testSh := fmt.Sprintf(`echo`)
			cmd := gosh.Command("bash", "-c", testSh)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Wait()).To(Succeed())
		})
	})
	When("using Kill()", func() {
		It("should stop the process", func() {
			testSh := fmt.Sprintf(`while true; sleep 3600; done`)
			cmd := gosh.Command("bash", "-c", testSh)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			Expect(cmd.Wait()).ToNot(Succeed())
		})
	})
})

var _ = Describe("PipelineCmd", func() {
	When("using not enough commands", func() {
		It("should fail", func() {
			Expect(gosh.Pipeline().Run()).ToNot(Succeed())
			Expect(gosh.Pipeline().Start()).ToNot(Succeed())
			Expect(gosh.Pipeline(gosh.Command("echo")).Run()).ToNot(Succeed())
			Expect(gosh.Pipeline(gosh.Command("echo")).Start()).ToNot(Succeed())
		})
	})

	When("killing the first command", func() {
		useMocks()
		It("should return the first command failed", func() {
			startMocks()
			cmd := gosh.Pipeline(
				gosh.Command("bash", "-c", "while true; do echo test; sleep 5; done"),
				gosh.Command("cat"),
			)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			Expect(cmd.Wait()).ToNot(Succeed())
		})
	})
	When("the first command fails", func() {
		useMocks()
		It("should return the first command failed", func() {
			startMocks()
			Expect(
				gosh.Pipeline(
					gosh.Command("exit", "1"),
					gosh.Command("cat"),
				).
					Run(),
			).ToNot(Succeed())
		})
	})
	When("the second command fails", func() {
		useMocks()
		It("should return the first command failed and stop the first command", func() {
			startMocks()
			cmd := gosh.Pipeline(
				gosh.Command("bash", "-c", "while true; do echo test; sleep 5; done"),
				gosh.Command("exit", "1"),
			)
			Expect(cmd.Run()).ToNot(Succeed())
		})
	})

	When("Forwarding in, out and err", func() {
		useMocks()

		It("should forward them", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						inOutPassthrough(),
						printStderr("err"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.ForwardAll),
				"in",
				"in",
				"err",
			)
		})
	})
	When("Forwarding in and err", func() {
		useMocks()

		It("should forward them", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
					)),
					gosh.Shell(allOf(
						checkStdin("This shouldn't be seen"),
						printStderr("err"),
					)),
				).WithStreams(gosh.ForwardInErr),
				"in",
				"",
				"err",
			)
		})
	})
	When("Forwarding in and out", func() {
		useMocks()

		It("should forward them", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
					gosh.Shell(allOf(
						inOutPassthrough(),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.ForwardInOut),
				"in",
				"out",
				"",
			)
		})
	})

	When("Forwarding out and err", func() {
		useMocks()

		It("should forward them but not in", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
						printStderr("err"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.ForwardOutErr),
				"This shouldn't be seen",
				"out",
				"err",
			)
		})
	})
	When("Forwarding in", func() {
		useMocks()

		It("should forward it but not out or err", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.ForwardIn),
				"in",
				"",
				"",
			)
		})
	})
	When("Forwarding out", func() {
		useMocks()

		It("should forward it but not in or err", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
					gosh.Shell(allOf(
						checkStdin("This shouldn't be seen"),
						printStdout("out"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.ForwardOut),
				"This shouldn't be seen",
				"out",
				"",
			)
		})
	})
	When("Forwarding err", func() {
		useMocks()

		It("should forward it but not in or out", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("This shouldn't be seen"),
					)),
					gosh.Shell(allOf(
						checkStdin("This shouldn't be seen"),
						printStderr("err"),
					)),
				).WithStreams(gosh.ForwardErr),
				"This shouldn't be seen",
				"",
				"err",
			)
		})
	})
	When("Processing in", func() {
		useMocks()

		It("should forward it", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("ininin"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					for ix := 0; ix < 3; ix++ {
						_, err := w.Write([]byte("in"))
						if err != nil {
							return err
						}
					}
					return nil
				})),
				"This shouldn't be seen",
				"",
				"",
			)
		})
	})
	When("Processing in fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericSyncFailTest(
				gosh.Pipeline(
					gosh.Shell(
						inOutPassthrough(),
					),
					gosh.Shell(
						inOutPassthrough(),
					),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					return err
				})),
				err,
			)
		})
	})
	When("Processing in fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericAsyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(
						inOutPassthrough(),
					),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					return err
				})),
				err,
			)
		})
	})

	When("Processing out", func() {
		useMocks()

		It("should forward them", func() {
			processedOut := "init"
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.FuncOut(gosh.AppendString(&processedOut))),
				"This shouldn't be seen",
				"",
				"",
			)
			Expect(processedOut).To(Equal("initout"))
		})
	})
	When("Processing out fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericSyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncOut(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})
	When("Processing out fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericAsyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncOut(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})

	When("Processing err", func() {
		useMocks()

		It("should forward them", func() {
			processedErr := []byte("init")
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FuncErr(gosh.AppendBytes(&processedErr))),
				"This shouldn't be seen",
				"",
				"",
			)
			Expect(string(processedErr)).To(Equal("initerr"))
		})
	})
	When("Processing err fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericSyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncErr(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})
	When("Processing err fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericAsyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncErr(func(r io.Reader) error {
					return err
				})),
				err,
			)
		})
	})
	When("Using a string for in", func() {
		useMocks()

		It("should send it", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.StringIn("in")),
				"",
				"",
				"",
			)
		})
	})
	When("Using a bytes for in", func() {
		useMocks()

		It("should send it", func() {
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.BytesIn([]byte("in"))),
				"",
				"",
				"",
			)
		})
	})
	When("Using a file for in", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			_, err = f.Write([]byte("in"))
			Expect(err).ToNot(HaveOccurred())
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FileIn(f.Name())),
				"",
				"",
				"",
			)
		})
	})
	When("Using a missing file for in", func() {
		useMocks()

		It("should fail", func() {
			genericSyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FileIn("This file doesn't exist")),
				os.ErrNotExist,
			)
		})
		It("should fail asynchronously", func() {
			genericAsyncFailStartTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FileIn("This file doesn't exist")),
				os.ErrNotExist,
			)
		})
	})

	When("Using a file for out", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
					)),
					gosh.Shell(allOf(
						inOutPassthrough(),
						printStderr("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileOut(f.Name())),
				"",
				"",
				"",
			)
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal("out"))
		})
	})
	When("Using an unwritable file for out", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericSyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.FileOut(f.Name())),
				os.ErrPermission,
			)
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal(""))
		})
	})
	When("Using an unwritable file for out asynchronously", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericAsyncFailStartTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileOut(f.Name())),
				os.ErrPermission,
			)
			actualStdout, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStdout)).To(Equal(""))
		})
	})

	When("Using a file for err", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			genericSyncTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileErr(f.Name())),
				"",
				"",
				"",
			)
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal("err"))
		})
	})
	When("Using an unwritable file for err", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericSyncFailTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileErr(f.Name())),
				os.ErrPermission,
			)
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal(""))
		})
	})
	When("Using an unwritable file for err asynchronously", func() {

		useMocks()

		It("should fail", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			st, err := f.Stat()
			Expect(err).ToNot(HaveOccurred())
			Expect(f.Chmod(st.Mode() &^ 0222)).To(Succeed())

			genericAsyncFailStartTest(
				gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileErr(f.Name())),
				os.ErrPermission,
			)
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal(""))
		})
	})

})
