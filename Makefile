Version := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
IsDirty := $(if $(shell git status --porcelain),-dirty,)
GitCommit := $(shell git rev-parse HEAD)
BuildTimestamp := $(shell date +%s)
LDFLAGS := "-s -w \
	-X oidc-cli/pkg.Version=$(Version)$(IsDirty) \
	-X oidc-cli/pkg.GitCommit=$(GitCommit) \
	-X oidc-cli/pkg.BuildTimestamp=$(BuildTimestamp)"
SOURCE_DIRS = . pkg
export GO111MODULE=on

.PHONY: all
all: gofmt test build dist hash

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags $(LDFLAGS) -o oidc-cli

.PHONY: gofmt
gofmt:
	@test -z $(shell gofmt -l -s $(SOURCE_DIRS) ./ | tee /dev/stderr) || (echo "[WARN] Fix formatting issues with 'make gofmt'" && exit 1)

.PHONY: test
test:
	CGO_ENABLED=0 go test $(shell go list ./... ) -cover

.PHONY: dist-local
dist-local:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags $(LDFLAGS) -o bin/oidc-cli

.PHONY: dist
dist:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -o bin/oidc-cli
	CGO_ENABLED=0 GOOS=darwin go build -ldflags $(LDFLAGS) -o bin/oidc-cli-darwin
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -a -ldflags $(LDFLAGS) -o bin/oidc-cli-darwin-arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -ldflags $(LDFLAGS) -o bin/oidc-cli-armhf
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags $(LDFLAGS) -o bin/oidc-cli-arm64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags $(LDFLAGS) -o bin/oidc-cli.exe

.PHONY: hash
hash:
	rm -rf bin/*.sha256 && ./hack/hashgen.sh

.PHONY: install
install: build
	sudo cp oidc-cli /usr/local/bin/

.PHONY: clean
clean:
	rm -rf bin oidc-cli
