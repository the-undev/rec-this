PREFIX  ?= /usr/local
BIN     := rec-this
SOURCES := $(wildcard *.go) go.mod

# make builds as the user; sudo make install only copies the binary it built.
build: $(BIN)

$(BIN): $(SOURCES)
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

install: $(BIN)
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)

uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/$(BIN)

clean:
	rm -f $(BIN)

.PHONY: build test lint fmt check install uninstall clean
