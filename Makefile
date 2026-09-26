# The front door. Each target is the one way to do its job.

.PHONY: help gates build test clean

help:
	@echo "make gates   gofmt + vet + test"
	@echo "make build   -> ./course"
	@echo "make test    go test ./..."
	@echo "make clean   remove ./course and empty .scratch/"

gates:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt: needs formatting:"; echo "$$out"; exit 1; fi; echo "gofmt: clean"
	@go vet ./... && echo "vet:   clean"
	@go test ./... >/dev/null && echo "test:  all packages ok" || (go test ./...; exit 1)

build:
	go build -o course ./cmd/course

test:
	go test ./...

clean:
	rm -f course
	rm -rf .scratch/*
