package gosh_test

import (
	"k8s.io/klog/v2"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGosh(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gosh Suite")
}

var _ = BeforeSuite(func() {
	klog.SetOutput(GinkgoWriter)
})
