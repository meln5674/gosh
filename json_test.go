package gosh_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meln5674/gosh"
)

type jsonTestStruct struct {
	A bool
	B int
	C string
	D []int
	E map[string]float64
	F interface{}
}

var _ = Describe("JSON", func() {
	It("should round-trip", func() {
		inObj := jsonTestStruct{
			A: true,
			B: 1,
			C: "foo",
			D: []int{1, 2, 3},
			E: map[string]float64{"bar": 0.5},
			F: nil,
		}
		var outObj jsonTestStruct
		Expect(gosh.
			Shell(inOutPassthrough()).
			WithStreams(
				gosh.FuncIn(gosh.JSONIn(&inObj)),
				gosh.FuncOut(gosh.SaveJSON(&outObj)),
			).
			Run(),
		).To(Succeed())
		Expect(outObj).To(Equal(inObj))

	})
})
