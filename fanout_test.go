//go:build linux

package gosh_test

import (
	"context"

	. "github.com/meln5674/gosh/pkg/gomega"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"fmt"
	"os"
	"os/exec"

	"github.com/meln5674/gosh"
)

var _ = Describe("FanOut", func() {
	When("running without a max concurrency", func() {
		It("should run all processes at once", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make([]gosh.Commander, 0, len(names))
			for _, name := range names {
				shells = append(shells, gosh.Shell(allOf(
					makeSentinel(dir, name),
					waitForSentinels(dir, names...),
				)))
			}
			Expect(gosh.FanOut(shells...).Run()).To(Succeed())
		})
	})
	When("Any process fails", func() {
		It("should fail", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make([]gosh.Commander, 0, 1+len(names))
			shells = append(shells, gosh.Shell(fail()))
			for _, name := range names {
				shells = append(shells, gosh.Shell(allOf(
					makeSentinel(dir, name),
					waitForSentinels(dir, names...),
				)))
			}
			Expect(gosh.FanOut(shells...).Run()).ToNot(Succeed())
		})
	})
	When("Any process fails to start", func() {
		It("should fail", func(ctx context.Context) {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make([]gosh.Commander, 0, 1+len(names))
			shells = append(shells, failToStart(ctx))
			for _, name := range names {
				shells = append(shells, gosh.Shell(allOf(
					makeSentinel(dir, name),
					waitForSentinels(dir, names...),
				)))
			}
			Expect(gosh.FanOut(shells...).Run()).ToNot(Succeed())
		})
	})
	When("running with a max concurrency", func() {
		It("should not run all processes at once", func() {
			dir, err := os.MkdirTemp("", "*")
			Expect(err).ToNot(HaveOccurred())
			defer os.RemoveAll(dir)
			names := []string{"a", "b", "c"}
			shells := make([]gosh.Commander, 0, len(names))
			for _, name := range names {
				shells = append(shells, gosh.Shell(allOf(
					makeSentinel(dir, name),
					waitForSentinels(dir, names...),
				)).UsingProcessGroup())
			}
			cmd := gosh.FanOut(shells...).WithMaxConcurrency(2)
			Expect(cmd.Start()).To(Succeed())
			Expect(gosh.Shell(waitForSentinels(dir, "a", "b")).Start()).To(Succeed())
			Expect(cmd.Kill()).To(Succeed())
			Expect(fmt.Sprintf("%s/%s", dir, "c")).ToNot(BeARegularFile())
			Expect(cmd.Wait()).To(Or(MatchMultiProcessError(gosh.ErrKilled), MatchMultiProcessErrorType(&exec.ExitError{})))
		})
	})
})
