SOURCE_DIRS=./

.PHONY: all
all: gofmt test build

.PHONY: build
build:
	CGO_ENABLED=0 go build -o oidc-cli -ldflags "-s -w" -v

.PHONY: gofmt
gofmt:
	@test -z $(shell gofmt -l -s $(SOURCE_DIRS) | tee /dev/stderr) || (echo "[WARN] Fix formatting issues with 'make gofmt'" && exit 1)

.PHONY: test
test:
	CGO_ENABLED=0 go test $(shell go list ./... ) -cover

.PHONY: install
install: build
	sudo cp oidc-cli /usr/local/bin/

.PHONY: clean
clean:
	rm -f oidc-cli
