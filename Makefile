BINARY=gsnote

.PHONY: build clean install

build:
	go build -o $(BINARY) ./cmd/bot

install:
	bash install.sh

clean:
	rm -f $(BINARY)

dev:
	go run ./cmd/bot
