## simple makefile to log workflow
.PHONY: all test clean build install

GOFLAGS ?= $(GOFLAGS:)
GOIMPORTS = goimports -w
PKG_LIST = pkg cmd main.go
FMT_LIST = $(foreach int, $(PKG_LIST), $(int))

all: install test

build:
	@go build $(GOFLAGS) ./...

docker:
	mkdir -p _output/bin && \
	go build $(GOFLAGS) -o _output/bin/custom-deployment main.go && \
	sudo docker build -t docker.io/mfojtik/custom-deployment:latest .

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
