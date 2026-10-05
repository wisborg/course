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

# -X stamps `git describe` into main.described, so a binary built from a
# checkout can say which release it is near instead of only "devel" plus a
# hash, which takes a second lookup to interpret. It is DERIVED, not written
# down: nobody has to remember to bump it. The same as fitdash and videofx.
#
# Deliberately not --dirty. A modified tree is already reported from the
# toolchain's own vcs.modified stamp, for every build however it was made,
# and asking describe for it too would print the fact twice.
#
# Empty if the repository has no tags yet, and empty for anyone running
# `go build` directly; version() falls back to the build information.
DESCRIBED := $(shell git describe --tags 2>/dev/null)

build:
	go build -ldflags "-X 'main.described=$(DESCRIBED)'" -o course ./cmd/course

test:
	go test ./...

clean:
	rm -f course
	rm -rf .scratch/*
