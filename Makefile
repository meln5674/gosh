GINKGO ?= bin/ginkgo
GINKGO_VERSION := v2.3.1

PHONY: ginkgo view-coverage lint

all: bin/coverage.html

bin/ginkgo:
	GOBIN=$(PWD)/bin/ go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

bin:
	mkdir -p bin

bin/coverage.out: test

view-coverage: bin/coverage.out
	go tool cover -html=bin/coverage.out

.PHONY: test
test: bin $(GINKGO) gosh.go $(wildcard *_test.go)
	$(GINKGO) run --trace --race -coverprofile=bin/coverage.out ./

lint:
	go vet ./...
	GOOS=windows go vet ./...
	golint
