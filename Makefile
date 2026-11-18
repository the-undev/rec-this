PREFIX ?= /usr/local
BIN    := rec-this

build:
	go build -trimpath -ldflags '-s -w' -o $(BIN) .

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: these files need formatting:"; echo "$$unformatted"; exit 1; \
	fi

check: fmt lint test build

install: build
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BIN)

clean:
	rm -f $(BIN)

.PHONY: build test lint fmt check install uninstall clean
