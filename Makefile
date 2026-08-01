# The gate for this repository's own work. CI runs `make check`, and so should
# you — one gate, not two.

GO      ?= go
VERSION ?= dev
LDFLAGS := -X github.com/ShiroDoromoto/weir/internal/version.Version=$(VERSION)

.PHONY: all check fmt vet test build clean

all: check

check: fmt vet test

fmt:
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "gofmt が要ります:"; \
		echo "$$files"; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/weir ./cmd/weir

clean:
	rm -rf bin
