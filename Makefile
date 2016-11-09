## simple makefile to log workflow
.PHONY: all test clean build install

GOFLAGS ?= $(GOFLAGS:)
GOIMPORTS = goimports -w
PKG_LIST = pkg cmd main.go
FMT_LIST = $(foreach int, $(PKG_LIST), $(int))

all: install test

build:
	@go build $(GOFLAGS) ./...

install:
	@go get $(GOFLAGS) ./...

test: install
	@go test $(GOFLAGS) ./...

bench: install
	@go test -run=NONE -bench=. $(GOFLAGS) ./...

clean:
	@go clean $(GOFLAGS) -i ./...

format:
	$(GOIMPORTS) $(FMT_LIST)

## EOF
