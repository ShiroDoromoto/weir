# The gate for this repository's own work. CI runs `make check`, and so should
# you — one gate, not two.

GO      ?= go
VERSION ?= dev
LDFLAGS := -X github.com/ShiroDoromoto/weir/internal/version.Version=$(VERSION)

.PHONY: all check fmt vet test actions build clean

all: check

check: fmt vet test actions

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

# ワークフローの壊れは、目で見ても出てこない。走らせて初めて分かる物を、走らせる前に見る。
# actionlint は go.mod の tool として固定してあるので、入れ忘れた誰かの手元だけ検査が
# 素通りする、ということが起きない。
actions:
	$(GO) tool actionlint -color

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/weir ./cmd/weir

clean:
	rm -rf bin
