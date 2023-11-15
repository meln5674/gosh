package gosh_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meln5674/gosh"
	. "github.com/meln5674/gosh/pkg/gomega"
)

var _ = Describe("And", func() {
	When("Nothing fails", func() {
		useMocks()
		It("should run all commands", func() {
			genericTest(genericTestArgs{
				cmd: gosh.And(
					gosh.Shell(printStdout("1")),
					gosh.Shell(printStdout("2")),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				stdin:  "",
				stdout: "123",
				stderr: "",
			})
		})
		It("should run all commands asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.And(
					gosh.Shell(printStdout("1")),
					gosh.Shell(printStdout("2")),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				async:  true,
				stdin:  "",
				stdout: "123",
				stderr: "",
			})
		})
	})
	When("Something fails", func() {
		useMocks()
		It("should not run commands after it", func() {
			genericTest(genericTestArgs{
				cmd: gosh.And(
					gosh.Shell(printStdout("1")),
					gosh.Shell(printStdout("2")),
					gosh.Shell(fail()),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				stdin:  "",
				stdout: "12",
				stderr: "",
				err:    &exec.ExitError{},
			})
		})
		It("should not run commands after it asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.And(
					gosh.Shell(printStdout("1")),
					gosh.Shell(printStdout("2")),
					gosh.Shell(fail()),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				async:  true,
				stdin:  "",
				stdout: "12",
				stderr: "",
				err:    &exec.ExitError{},
			})
		})
	})
	When("Processing out", func() {
		useMocks()
		It("should work as expected", func() {
			var processedOut []byte
			genericTest(genericTestArgs{
				cmd: gosh.And(
					gosh.Shell(printStdout("1")),
					gosh.Shell(printStdout("2")),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.FuncOut(gosh.SaveBytes(&processedOut))),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
			Expect(string(processedOut)).To(Equal("123"))
		})
		It("should work as expected asynchronously", func() {
			processedOut := []byte("init")
			genericTest(genericTestArgs{
				cmd: gosh.And(
					gosh.Shell(printStdout("1")),
					gosh.Shell(printStdout("2")),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.FuncOut(gosh.AppendBytes(&processedOut))),
				async:  true,
				stdin:  "",
				stdout: "",
				stderr: "",
			})
			Expect(string(processedOut)).To(Equal("init123"))
		})
	})
	When("killing", func() {
		useMocks()
		It("should not run later commands", func() {
			cmd := gosh.And(
				gosh.Command("sleep", "3600"),
				gosh.Shell(printStdout("This shouldn't be seen")),
			).WithStreams(gosh.ForwardOut)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			err := cmd.Wait()
			Expect(err).To(MatchMultiProcessError(gosh.ErrKilled))
			Expect(mockStdout.String()).To(Equal(""))
		})
	})
})

var _ = Describe("Or", func() {
	When("Everything fails", func() {
		useMocks()
		It("should run all commands and fail", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStdout("1"), fail())),
					gosh.Shell(allOf(printStdout("2"), fail())),
					gosh.Shell(allOf(printStdout("3"), fail())),
				).WithStreams(gosh.ForwardOut),
				stdin:  "",
				stdout: "123",
				stderr: "",
				err:    &exec.ExitError{},
			})
		})
		It("should run all commands and fail asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStdout("1"), fail())),
					gosh.Shell(allOf(printStdout("2"), fail())),
					gosh.Shell(allOf(printStdout("3"), fail())),
				).WithStreams(gosh.ForwardOut),
				async:  true,
				stdin:  "",
				stdout: "123",
				stderr: "",
				err:    &exec.ExitError{},
			})
		})
	})
	When("Something succeeds", func() {
		useMocks()
		It("should not run commands after it", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStdout("1"), fail())),
					gosh.Shell(printStdout("2")),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				stdin:  "",
				stdout: "12",
				stderr: "",
			})
		})
		It("should not run commands after it asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStdout("1"), fail())),
					gosh.Shell(printStdout("2")),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				async:  true,
				stdin:  "",
				stdout: "12",
				stderr: "",
			})
		})
	})
	When("The last command succeeds", func() {
		useMocks()
		It("should report success", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStdout("1"), fail())),
					gosh.Shell(allOf(printStdout("2"), fail())),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				stdin:  "",
				stdout: "123",
				stderr: "",
			})
		})
		It("report success after it asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStdout("1"), fail())),
					gosh.Shell(allOf(printStdout("2"), fail())),
					gosh.Shell(printStdout("3")),
				).WithStreams(gosh.ForwardOut),
				async:  true,
				stdin:  "",
				stdout: "123",
				stderr: "",
			})
		})
	})
	When("Processing err", func() {
		useMocks()
		It("should work as expected", func() {
			var processedErr string
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStderr("1"), fail())),
					gosh.Shell(allOf(printStderr("2"), fail())),
					gosh.Shell(allOf(printStderr("3"), fail())),
				).WithStreams(gosh.FuncErr(gosh.SaveString(&processedErr))),
				stdin:  "",
				stdout: "",
				stderr: "",
				err:    &exec.ExitError{},
			})
			Expect(processedErr).To(Equal("123"))
		})
		It("should work as expected asynchronously", func() {
			processedErr := "init"
			genericTest(genericTestArgs{
				cmd: gosh.Or(
					gosh.Shell(allOf(printStderr("1"), fail())),
					gosh.Shell(allOf(printStderr("2"), fail())),
					gosh.Shell(allOf(printStderr("3"), fail())),
				).WithStreams(gosh.FuncErr(gosh.AppendString(&processedErr))),
				async:  true,
				stdin:  "",
				stdout: "",
				stderr: "",
				err:    &exec.ExitError{},
			})
			Expect(processedErr).To(Equal("init123"))
		})
	})
	When("killing", func() {
		useMocks()
		It("should not run later commands", func() {
			cmd := gosh.Or(
				gosh.Command("sleep", "3600"),
				gosh.Shell(printStdout("This shouldn't be seen")),
			).WithStreams(gosh.ForwardOut)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			err := cmd.Wait()
			Expect(err).To(MatchMultiProcessError(gosh.ErrKilled))
			Expect(mockStdout.String()).To(Equal(""))
		})
	})
})

var _ = Describe("Then", func() {
	When("Everything fails", func() {
		useMocks()
		It("should run all commands", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Then(
					gosh.Shell(allOf(printStderr("1"), fail())),
					gosh.Shell(allOf(printStderr("2"), fail())),
					gosh.Shell(allOf(printStderr("3"), fail())),
				).WithStreams(gosh.ForwardErr),
				stdin:  "",
				stdout: "",
				stderr: "123",
				err:    &exec.ExitError{},
			})
		})
		It("should run all commands asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Then(
					gosh.Shell(allOf(printStderr("1"), fail())),
					gosh.Shell(allOf(printStderr("2"), fail())),
					gosh.Shell(allOf(printStderr("3"), fail())),
				).WithStreams(gosh.ForwardErr),
				async:  true,
				stdin:  "",
				stdout: "",
				stderr: "123",
				err:    &exec.ExitError{},
			})
		})
	})
	When("Everything succeeds", func() {
		useMocks()
		It("should run all commands", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Then(
					gosh.Shell(checkStdinLine("1")),
					gosh.Shell(checkStdinLine("2")),
					gosh.Shell(checkStdinLine("3")),
				).WithStreams(gosh.ForwardIn),
				stdin:  "1\n2\n3\n",
				stdout: "",
				stderr: "",
			})
		})
		It("should run all commands asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Then(
					gosh.Shell(checkStdinLine("1")),
					gosh.Shell(checkStdinLine("2")),
					gosh.Shell(checkStdinLine("3")),
				).WithStreams(gosh.ForwardIn),
				async:  true,
				stdin:  "1\n2\n3\n",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("Using literal input", func() {
		useMocks()
		It("should work as expected", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Then(
					gosh.Shell(checkStdinChar('1')),
					gosh.Shell(checkStdinChar('2')),
					gosh.Shell(checkStdinChar('3')),
				).WithStreams(gosh.StringIn("123"), gosh.ForwardOut, gosh.ForwardErr),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
		})
		It("should work as expected asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.Then(
					gosh.Shell(checkStdinChar('1')),
					gosh.Shell(checkStdinChar('2')),
					gosh.Shell(checkStdinChar('3')),
				).WithStreams(gosh.BytesIn([]byte("123"))),
				async:  true,
				stdin:  "",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("killing", func() {
		useMocks()
		It("should not run later commands", func() {
			cmd := gosh.Then(
				gosh.Command("sleep", "3600"),
				gosh.Shell(printStdout("This shouldn't be seen")),
			).WithStreams(gosh.ForwardOut)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			err := cmd.Wait()
			Expect(err).To(MatchMultiProcessError(gosh.ErrKilled))
			Expect(mockStdout.String()).To(Equal(""))
		})
	})
})

var _ = Describe("All", func() {
	When("Everything fails", func() {
		useMocks()
		It("should run all commands", func() {
			genericTest(genericTestArgs{
				cmd: gosh.All(
					gosh.Shell(allOf(printStderr("1"), fail())),
					gosh.Shell(allOf(printStderr("2"), fail())),
					gosh.Shell(allOf(printStderr("3"), fail())),
				).WithStreams(gosh.ForwardErr),
				stdin:  "",
				stdout: "",
				stderr: "123",
				err:    &gosh.MultiProcessError{},
			})
		})
		It("should run all commands asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.All(
					gosh.Shell(allOf(printStderr("1"), fail())),
					gosh.Shell(allOf(printStderr("2"), fail())),
					gosh.Shell(allOf(printStderr("3"), fail())),
				).WithStreams(gosh.ForwardErr),
				async:  true,
				stdin:  "",
				stdout: "",
				stderr: "123",
				err:    &gosh.MultiProcessError{},
			})
		})
	})
	When("Everything succeeds", func() {
		useMocks()
		It("should run all commands", func() {
			genericTest(genericTestArgs{
				cmd: gosh.All(
					gosh.Shell(checkStdinLine("1")),
					gosh.Shell(checkStdinLine("2")),
					gosh.Shell(checkStdinLine("3")),
				).WithStreams(gosh.ForwardIn),
				stdin:  "1\n2\n3\n",
				stdout: "",
				stderr: "",
			})
		})
		It("should run all commands asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.All(
					gosh.Shell(checkStdinLine("1")),
					gosh.Shell(checkStdinLine("2")),
					gosh.Shell(checkStdinLine("3")),
				).WithStreams(gosh.ForwardIn),
				async:  true,
				stdin:  "1\n2\n3\n",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("Using literal input", func() {
		useMocks()
		It("should work as expected", func() {
			genericTest(genericTestArgs{
				cmd: gosh.All(
					gosh.Shell(checkStdinChar('1')),
					gosh.Shell(checkStdinChar('2')),
					gosh.Shell(checkStdinChar('3')),
				).WithStreams(gosh.StringIn("123"), gosh.ForwardOut, gosh.ForwardErr),
				stdin:  "",
				stdout: "",
				stderr: "",
			})
		})
		It("should work as expected asynchronously", func() {
			genericTest(genericTestArgs{
				cmd: gosh.All(
					gosh.Shell(checkStdinChar('1')),
					gosh.Shell(checkStdinChar('2')),
					gosh.Shell(checkStdinChar('3')),
				).WithStreams(gosh.BytesIn([]byte("123"))),
				async:  true,
				stdin:  "",
				stdout: "",
				stderr: "",
			})
		})
	})
	When("killing", func() {
		useMocks()
		It("should not run later commands", func() {
			cmd := gosh.All(
				gosh.Command("sleep", "3600"),
				gosh.Shell(printStdout("This shouldn't be seen")),
			).WithStreams(gosh.ForwardOut)
			Expect(cmd.Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			err := cmd.Wait()
			Expect(err).To(MatchMultiProcessError(gosh.ErrKilled))
			Expect(mockStdout.String()).To(Equal(""))
		})
	})
})
