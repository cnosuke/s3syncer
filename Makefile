NAME     := s3syncer
VERSION  := v0.0.1
REVISION := $(shell git rev-parse --short HEAD)
SRCS    := $(shell find . -type f -name '*.go')
LDFLAGS := -ldflags="-s -w -X \"main.Version=$(VERSION)\" -X \"main.Revision=$(REVISION)\" -extldflags \"-static\""

bin/$(NAME): $(SRCS)
	go build -a -tags netgo -installsuffix netgo $(LDFLAGS) -o bin/$(NAME)

.PHONY: clean
clean:
	rm -rf bin/* dist/*

.PHONY: cross-build
cross-build:
	for os in darwin linux windows; do \
		for arch in amd64 386; do \
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -a -tags netgo -installsuffix netgo $(LDFLAGS) -o dist/$$os-$$arch/$(NAME); \
		done; \
	done

.PHONY: release-pack
release-pack: cross-build
	for os in darwin linux windows; do \
		for arch in amd64 386; do \
			zip -j dist/$$os-$$arch.zip dist/$$os-$$arch/$(NAME); \
		done; \
	done