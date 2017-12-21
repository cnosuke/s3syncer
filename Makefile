NAME     := s3syncer
VERSION  := v0.0.3
REVISION := $(shell git rev-parse --short HEAD)
SRCS    := $(shell find . -type f -name '*.go')
LDFLAGS := -ldflags="-s -w -X \"main.Version=$(VERSION)\" -X \"main.Revision=$(REVISION)\" -extldflags \"-static\""

.PHONY: clean cross-build release-pack glide deps

bin/$(NAME): $(SRCS)
	go build -a -tags netgo -installsuffix netgo $(LDFLAGS) -o bin/$(NAME)

clean:
	rm -rf bin/* dist/* vendor/*

cross-build:
	for os in darwin linux windows; do \
		for arch in amd64 386; do \
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -a -tags netgo -installsuffix netgo $(LDFLAGS) -o dist/$$os-$$arch/$(NAME); \
		done; \
	done

release-pack: cross-build
	for os in darwin linux windows; do \
		for arch in amd64 386; do \
			zip -j dist/$$os-$$arch.zip dist/$$os-$$arch/$(NAME); \
		done; \
	done

glide:
ifeq ($(shell command -v glide 2> /dev/null),)
	curl https://glide.sh/get | sh
endif

deps: glide
	glide install -v
