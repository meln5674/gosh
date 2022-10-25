//go:build !linux

package gosh_test

import (
	. "github.com/meln5674/gosh/pkg/gomega"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"fmt"
	"os"
	"os/exec"

	"github.com/meln5674/gosh"
)

var _ = Describe("Queue", func() {
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
					))
					cmds = append(cmds, cmd)
					shells <- cmd
				}
				close(shells)
			}()
			Expect(gosh.Shell(waitForSentinels(dir, "a", "b")).Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			Expect(fmt.Sprintf("%s/%s", dir, "c")).ToNot(BeARegularFile())
			Expect(cmd.Wait()).To(Or(MatchMultiProcessError(gosh.ErrKilled), MatchMultiProcessErrorType(&exec.ExitError{})))
			Expect(cmds[0].ProcessState).ToNot(BeNil())
			Expect(cmds[1].ProcessState).ToNot(BeNil())
			Expect(cmds[2].ProcessState).To(BeNil())
		})
	})
})
