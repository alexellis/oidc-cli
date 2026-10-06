.PHONY: build clean check

build:
	go build -o oidc-cli .

install: build
	sudo cp oidc-cli /usr/local/bin/

check: build
	./oidc-cli -h

clean:
	rm -f oidc-cli
