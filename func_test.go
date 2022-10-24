package gosh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"context"
	"github.com/meln5674/gosh"
	"io"
	"io/ioutil"
	"os"
)

var _ = Describe("FuncCmd", func() {
	useMocks()
	It("should work like you expect", func() {
		genericTest(genericTestArgs{
			cmd: gosh.FromFunc(
				context.TODO(),
				func(
					ctx context.Context,
					stdin io.Reader, stdout, stderr io.Writer,
					done chan error,
				) error {
					go func() {
						defer close(done)
						_, err := io.Copy(stdout, stdin)
						done <- err
					}()
					go func() {
						stderr.Write([]byte("err"))
					}()
					return nil
				},
			).WithStreams(gosh.ForwardAll),
			stdin:  "in",
			stdout: "in",
			stderr: "err",
		})
	})
	It("should work like you expect asynchronously", func() {
		genericTest(genericTestArgs{
			cmd: gosh.FromFunc(
				context.TODO(),
				func(
					ctx context.Context,
					stdin io.Reader, stdout, stderr io.Writer,
					done chan error,
				) error {
					go func() {
						defer close(done)
						_, err := io.Copy(stdout, stdin)
						done <- err
					}()
					go func() {
						stderr.Write([]byte("err"))
					}()
					return nil
				},
			).WithStreams(gosh.ForwardAll),
			stdin:  "in",
			stdout: "in",
			stderr: "err",
			async:  true,
		})
	})
	It("should be killable", func() {
		cmd := gosh.FromFunc(
			context.TODO(),
			func(
				ctx context.Context,
				stdin io.Reader, stdout, stderr io.Writer,
				done chan error,
			) error {
				go func() {
					for range ctx.Done() {
					}
					done <- ctx.Err()
				}()
				return nil
			},
		).WithStreams(gosh.ForwardAll)
		Expect(cmd.Start()).To(Succeed())
		Expect(cmd.Kill()).To(Succeed())
		Expect(cmd.Wait()).To(MatchError(gosh.ErrKilled))
	})
	It("should work as part of a pipeline", func() {
		f, err := os.CreateTemp("", "*")
		Expect(err).ToNot(HaveOccurred())
		defer os.Remove(f.Name())
		genericTest(genericTestArgs{
			cmd: gosh.Pipeline(
				gosh.FromFunc(
					context.TODO(),
					func(
						ctx context.Context,
						stdin io.Reader, stdout, stderr io.Writer,
						done chan error,
					) error {
						go func() {
							defer close(done)
							_, err := io.Copy(stdout, stdin)
							done <- err
						}()
						return nil
					},
				),
				gosh.FromFunc(
					context.TODO(),
					func(
						ctx context.Context,
						stdin io.Reader, stdout, stderr io.Writer,
						done chan error,
					) error {
						go func() {
							defer close(done)
							_, err := io.Copy(stdout, stdin)
							done <- err
						}()
						go func() {
							stderr.Write([]byte("err"))
						}()
						return nil
					},
				),
			).WithStreams(
				gosh.ForwardIn,
				gosh.FileOut(f.Name()),
				gosh.ForwardErr,
			),
			stdin:  "in",
			stdout: "",
			stderr: "err",
			async:  true,
		})
		processedOut, err := ioutil.ReadFile(f.Name())
		Expect(err).ToNot(HaveOccurred())
		Expect(string(processedOut)).To(Equal("in"))
	})
})
