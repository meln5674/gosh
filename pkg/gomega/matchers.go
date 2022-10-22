package gomega

import (
	"errors"
	"fmt"
	"github.com/meln5674/gosh"
	g "github.com/onsi/gomega"
	gformat "github.com/onsi/gomega/format"
	gtypes "github.com/onsi/gomega/types"
)

type MultiProcessErrorMatcher struct {
	Expected error
}

// MatchMultiProcessError succeeds if the actual error is a MultiProcessError
// and errors.Is matches the expected error with any of the errors in it recursively
func MatchMultiProcessError(expected error) gtypes.GomegaMatcher {
	return &MultiProcessErrorMatcher{Expected: expected}
}

func (m *MultiProcessErrorMatcher) Match(actual interface{}) (success bool, err error) {
	//return m.cachedInner(actual).Match(actual)

	check := g.HaveOccurred()
	success, err = check.Match(actual)
	if !success || err != nil {
		return
	}
	check = g.MatchError(m.Expected)
	success, err = check.Match(actual)
	if err != nil {
		return
	}
	if success {
		return
	}
	check = g.BeAssignableToTypeOf(&gosh.MultiProcessError{})
	success, err = check.Match(actual)
	if !success || err != nil {
		return
	}
	multi := new(gosh.MultiProcessError)
	errors.As(actual.(error), &multi)
	for _, subActual := range multi.Errors {
		check = m
		success, err = check.Match(subActual)
		if success {
			return true, nil
		}
		if err != nil {
			return
		}
	}
	return false, nil
}

func (m *MultiProcessErrorMatcher) FailureMessage(actual interface{}) (message string) {
	check := g.HaveOccurred()
	success, _ := check.Match(actual)
	if !success {
		return check.FailureMessage(actual)
	}
	check = g.MatchError(m.Expected)
	success, _ = check.Match(actual)
	if success {
		panic("Impossible state")
	}
	check = g.BeAssignableToTypeOf(&gosh.MultiProcessError{})
	success, _ = check.Match(actual)
	if !success {
		return check.FailureMessage(actual)
	}

	return fmt.Sprintf(
		"Expected\n%sto be somewhere within the tree of\n%s",
		gformat.Object(m.Expected, 1),
		gformat.Object(actual, 1),
	)
}
func (m *MultiProcessErrorMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	check := g.HaveOccurred()
	success, _ := check.Match(actual)
	if success {
		return check.NegatedFailureMessage(actual)
	}
	check = g.MatchError(m.Expected)
	success, _ = check.Match(actual)
	if success {
		return check.NegatedFailureMessage(actual)
	}
	check = g.BeAssignableToTypeOf(&gosh.MultiProcessError{})
	success, _ = check.Match(actual)
	if success {
		return check.NegatedFailureMessage(actual)
	}

	return fmt.Sprintf(
		"Expected\n%sto be nowhere within the tree of\n%s",
		gformat.Object(m.Expected, 1),
		gformat.Object(actual, 1),
	)

}
