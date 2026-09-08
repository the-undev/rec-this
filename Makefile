PREFIX ?= $(HOME)/.local
BIN    := rec

build:
	go build -trimpath -ldflags '-s -w' -o $(BIN) .

install: build
	install -Dm755 $(BIN) $(DESTDIR)$(PREFIX)/bin/$(BIN)
	@echo "installed -> $(DESTDIR)$(PREFIX)/bin/$(BIN)"

clean:
	rm -f $(BIN)

.PHONY: build install clean
