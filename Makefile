.SILENT:
.PHONY: help gen proto clean dist build

NAME:=strata
ROOF:=github.com/liut/strata
DATE:=$(shell date '+%Y%m%d')
TAG:=$(shell git describe --tags --always 2>/dev/null || echo "dev")
GO:=$(shell which go)
GOMOD:=$(shell echo "$${GO111MODULE:-auto}")

# VERSION can be overridden: make dist VERSION=v1.2.3
VERSION?=$(DATE)-$(TAG)

LDFLAGS:=-X $(ROOF)/cmd.version=$(VERSION)

help:
	@echo "Usage:"
	@echo "  make gen          - Generate protobuf code"
	@echo "  make dist         - Build for all platforms"
	@echo "  make dist VERSION=xxx - Build with custom version"
	@echo "  make build        - Build for local platform"
	@echo "  make clean        - Clean build artifacts"

PROTO_DIR:=pkg/proto/sandbox

gen proto:
	cd $(PROTO_DIR) && protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		sandbox.proto

vet:
	echo "Checking ./pkg/... ./cmd/... , with GOMOD=$(GOMOD)"
	GO111MODULE=$(GOMOD) $(GO) vet -all ./pkg/...

build:
	GO111MODULE=$(GOMOD) $(GO) build -ldflags "$(LDFLAGS)" -o $(NAME) .

showver:
	echo "version: $(DATE)-$(TAG)"

dist/linux_amd64/$(NAME): $(SOURCES) showver
	echo "Building $(NAME) of linux/x64"
	mkdir -p dist/linux_amd64 && GO111MODULE=$(GOMOD) GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS) -s -w" -o dist/linux_amd64/$(NAME) .

dist/darwin_amd64/$(NAME): $(SOURCES) showver
	echo "Building $(NAME) of darwin/x64"
	mkdir -p dist/darwin_amd64 && GO111MODULE=$(GOMOD) GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS) -w" -o dist/darwin_amd64/$(NAME) .

dist/darwin_arm64/$(NAME): $(SOURCES) showver
	echo "Building $(NAME) of darwin/arm64"
	mkdir -p dist/darwin_arm64 && GO111MODULE=$(GOMOD) GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS) -w" -o dist/darwin_arm64/$(NAME) .

dist: vet dist/linux_amd64/$(NAME) dist/darwin_amd64/$(NAME) dist/darwin_arm64/$(NAME)

package-linux: dist/linux_amd64/$(NAME)
	echo "Packaging $(NAME)"
	ls dist/linux_amd64 | xargs tar -cvJf $(NAME)-linux-amd64-$(DATE)-$(TAG).tar.xz -C dist/linux_amd64

clean:
	rm -rf dist
	rm -f $(NAME) $(NAME)-*
