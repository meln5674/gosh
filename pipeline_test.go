package gosh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"errors"
	"io"
	"io/ioutil"
	"os"

	"github.com/meln5674/gosh"
)

var _ = Describe("PipelineCmd", func() {
	When("using not enough commands", func() {
		It("should fail", func() {
			Expect(gosh.Pipeline().Run()).ToNot(Succeed())
			Expect(gosh.Pipeline().Start()).ToNot(Succeed())
			Expect(gosh.Pipeline(gosh.Command("echo")).Run()).ToNot(Succeed())
			Expect(gosh.Pipeline(gosh.Command("echo")).Start()).ToNot(Succeed())
		})
	})

	When("Using more than 2 commands", func() {
		useMocks()

		It("should work as expected", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(`sed -E 's/(.*)/x\1/'`),
					gosh.Shell(`sed -E 's/(.*)/y\1/'`),
					gosh.Shell(`sed -E 's/(.*)/z\1/'`),
				).WithStreams(gosh.ForwardAll),
				stdin:  "1\n2\n3\n",
				stdout: "zyx1\nzyx2\nzyx3\n",
				stderr: "",
			})
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
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(allOf(
						inOutPassthrough(),
						printStderr("err"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.ForwardAll),
				stdin:  "in",
				stdout: "in",
				stderr: "err",
			})
		})
	})
	When("Forwarding in and err", func() {
		useMocks()

		It("should forward them", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
					)),
					gosh.Shell(allOf(
						checkStdin("This shouldn't be seen"),
						printStderr("err"),
					)),
				).WithStreams(gosh.ForwardInErr),
				stdin:  "in",
				stdout: "",
				stderr: "err",
			})
		})
	})
	When("Forwarding in and out", func() {
		useMocks()

		It("should forward them", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
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
				stdin:  "in",
				stdout: "out",
				stderr: "",
			})
		})
	})

	When("Forwarding out and err", func() {
		useMocks()

		It("should forward them but not in", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
						printStderr("err"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.ForwardOutErr),
				stdin:  "This shouldn't be seen",
				stdout: "out",
				stderr: "err",
			})
		})
	})
	When("Forwarding in", func() {
		useMocks()

		It("should forward it but not out or err", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.ForwardIn),
				stdin:  "in",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("Forwarding out", func() {
		useMocks()

		It("should forward it but not in or err", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
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
				stdin:  "This shouldn't be seen",
				stdout: "out",
				stderr: "",
			})
		})
	})
	When("Forwarding err", func() {
		useMocks()

		It("should forward it but not in or out", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("This shouldn't be seen"),
					)),
					gosh.Shell(allOf(
						checkStdin("This shouldn't be seen"),
						printStderr("err"),
					)),
				).WithStreams(gosh.ForwardErr),
				stdin:  "This shouldn't be seen",
				stdout: "",
				stderr: "err",
			})
		})
	})
	When("Processing in", func() {
		useMocks()

		It("should forward it", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
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
				stdin:  "This shouldn't be seen",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("Processing in fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(
						inOutPassthrough(),
					),
					gosh.Shell(
						inOutPassthrough(),
					),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					return err
				})),
				err: err,
			})
		})
	})
	When("Processing in fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(
						inOutPassthrough(),
					),
				).WithStreams(gosh.FuncIn(func(w io.Writer) error {
					return err
				})),
				err:   err,
				async: true,
			})
		})
	})

	When("Processing out", func() {
		useMocks()

		It("should forward them", func() {
			processedOut := "init"
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(allOf(
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.FuncOut(gosh.AppendString(&processedOut))),
				stdin:  "This shouldn't be seen",
				stdout: "",
				stderr: "",
			})
			Expect(processedOut).To(Equal("initout"))
		})
	})
	When("Processing out fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncOut(func(r io.Reader) error {
					return err
				})),
				err: err,
			})
		})
	})
	When("Processing out fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncOut(func(r io.Reader) error {
					return err
				})),
				err: err,
			})
		})
	})

	When("Processing err", func() {
		useMocks()

		It("should forward them", func() {
			processedErr := []byte("init")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FuncErr(gosh.AppendBytes(&processedErr))),
				stdin:  "This shouldn't be seen",
				stdout: "",
				stderr: "",
			})
			Expect(string(processedErr)).To(Equal("initerr"))
		})
	})
	When("Processing err fails", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncErr(func(r io.Reader) error {
					return err
				})),
				err: err,
			})
		})
	})
	When("Processing err fails asynchronously", func() {
		useMocks()

		It("should return the error", func() {
			err := errors.New("This is an error")
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FuncErr(func(r io.Reader) error {
					return err
				})),
				err:   err,
				async: true,
			})
		})
	})
	When("Using a string for in", func() {
		useMocks()

		It("should send it", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.StringIn("in")),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("Using a bytes for in", func() {
		useMocks()

		It("should send it", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.BytesIn([]byte("in"))),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
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
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin("in"),
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FileIn(f.Name())),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("Using a missing file for in", func() {
		useMocks()

		It("should fail", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FileIn("This file doesn't exist")),
				err: os.ErrNotExist,
			})
		})
		It("should fail asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						printStdout("This shouldn't be seen"),
						printStderr("Neither should this"),
					)),
				).WithStreams(gosh.FileIn("This file doesn't exist")),
				err:        os.ErrNotExist,
				errOnStart: true,
				async:      true,
			})
		})
	})

	When("Using a file for out", func() {
		useMocks()

		It("should forward them", func() {
			f, err := os.CreateTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(f.Name())
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
					)),
					gosh.Shell(allOf(
						inOutPassthrough(),
						printStderr("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileOut(f.Name())),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
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

			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
					gosh.Shell(inOutPassthrough()),
				).WithStreams(gosh.FileOut(f.Name())),
				err: os.ErrPermission,
			})
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

			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStdout("out"),
						printStderr("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileOut(f.Name())),
				err:        os.ErrPermission,
				errOnStart: true,
				async:      true,
			})
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
			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileErr(f.Name())),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
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

			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileErr(f.Name())),
				err: os.ErrPermission,
			})
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

			genericTest(genericTestArgs{
				cmd: gosh.Pipeline(
					gosh.Shell(inOutPassthrough()),
					gosh.Shell(allOf(
						checkStdin(""),
						printStderr("err"),
						printStdout("This shouldn't be seen"),
					)),
				).WithStreams(gosh.FileErr(f.Name())),
				err:        os.ErrPermission,
				errOnStart: true,
				async:      true,
			})
			actualStderr, err := ioutil.ReadFile(f.Name())
			Expect(err).ToNot(HaveOccurred())
			Expect(string(actualStderr)).To(Equal(""))
		})
	})

})
