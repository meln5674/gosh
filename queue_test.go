//go:build linux

package gosh_test

import (
	"context"
	"path/filepath"
	"time"

	. "github.com/meln5674/gosh/pkg/gomega"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"fmt"
	"os"
	"os/exec"

	"github.com/meln5674/gosh"
)

var _ = Describe("Queue", func() {
	When("running invalid max concurrency", func() {
		It("should fail to start", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make(chan gosh.Commander, len(names))
			cmd := gosh.Queue(shells).WithMaxConcurrency(0).WithResultBufferSize(3)
			Expect(cmd.Start()).NotTo(Succeed())
		})
		It("should fail to start", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make(chan gosh.Commander, len(names))
			cmd := gosh.Queue(shells).WithMaxConcurrency(0).WithResultBufferSize(3)
			Expect(cmd.Run()).NotTo(Succeed())
		})
	})
	When("running without a max concurrency", func() {
		It("should run all processes at once", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make(chan gosh.Commander)
			go func() {
				defer GinkgoRecover()
				for _, name := range names {
					shells <- gosh.Shell(allOf(
						makeSentinel(dir, name),
						waitForSentinels(dir, names...),
					))
				}
				close(shells)
			}()
			Expect(gosh.Queue(shells).Run()).To(Succeed())
		})
	})
	When("running with a max concurrency", func() {
		It("should not run all processes at once", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make(chan gosh.Commander, len(names))
			cmd := gosh.Queue(shells).WithMaxConcurrency(2).WithResultBufferSize(3)
			Expect(cmd.Start()).To(Succeed())
			cmds := make([]*gosh.Cmd, 0, 3)
			go func() {
				for _, name := range names {
					cmd := gosh.Shell(allOf(
						makeSentinel(dir, name),
						waitForSentinels(dir, names...),
					)).UsingProcessGroup()
					cmds = append(cmds, cmd)
					shells <- cmd
				}
				close(shells)
			}()
			Expect(gosh.Shell(waitForSentinels(dir, "a", "b")).Start()).To(Succeed())
			// If we don't sleep here, there is a chance that the processes don't have time to register their state
			time.Sleep(1 * time.Second)
			Expect(cmd.Kill()).To(Succeed())
			Expect(fmt.Sprintf("%s/%s", dir, "c")).ToNot(BeARegularFile())
			Expect(cmd.Wait()).To(Or(MatchMultiProcessError(gosh.ErrKilled), MatchMultiProcessErrorType(&exec.ExitError{})))
			Expect(cmds[0].ProcessState).ToNot(BeNil())
			Expect(cmds[1].ProcessState).ToNot(BeNil())
			Expect(cmds[2].ProcessState).To(BeNil())
		})
	})
	When("Any process fails", func() {
		It("should fail", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make(chan gosh.Commander)
			go func() {
				defer GinkgoRecover()
				for _, name := range names {
					shells <- gosh.Shell(allOf(
						makeSentinel(dir, name),
						waitForSentinels(dir, names...),
					))
				}
				shells <- gosh.Shell(fail())
				close(shells)
			}()
			Expect(gosh.Queue(shells).Run()).ToNot(Succeed())
		})
	})
	When("Any process fails to start", func() {
		It("should fail", func(ctx context.Context) {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make(chan gosh.Commander)
			go func() {
				defer GinkgoRecover()
				for _, name := range names {
					shells <- gosh.Shell(allOf(
						makeSentinel(dir, name),
						waitForSentinels(dir, names...),
					))
				}
				shells <- failToStart(ctx)
				close(shells)
			}()
			Expect(gosh.Queue(shells).Run()).ToNot(Succeed())
		})
	})
	It("Should not complete until all processes are done", func(ctx context.Context) {
		dir, err := os.MkdirTemp("", "*")
		Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(dir)
		shells := make(chan gosh.Commander, 1)
		shells <- gosh.Shell("sleep 10")
		sentinel := filepath.Join(dir, "a")
		go func() {
			defer GinkgoRecover()
			Expect(gosh.And(gosh.Queue(shells), gosh.Shell(makeSentinel(dir, "a"))).Run()).To(Succeed())
		}()
		Consistently(func() error {
			_, err := os.Stat(sentinel)
			return err
		}, "200ms", "8s").Should(MatchError(os.ErrNotExist))
	})
})
