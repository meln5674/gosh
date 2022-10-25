GINKGO_VERSION := v2.3.1

PHONY: ginkgo view-coverage lint

all: coverage.html

ginkgo:
	which ginko || (cd ~; go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION))

bin:
	mkdir -p bin

bin/coverage.out: bin ginkgo gosh.go $(wildcard *_test.go)
	go test -v -coverprofile=bin/coverage.out ./

bin/coverage.html: bin/coverage.out
	go tool cover -html=bin/coverage.out

view-coverage: bin/coverage.html
	xdg-open bin/coverage.html

lint:
	go vet ./...
	GOOS=windows go vet ./...
	golint
