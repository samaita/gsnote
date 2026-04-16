BINARY=gsnote

.PHONY: build clean

build:
	go build -o $(BINARY) ./cmd/bot

clean:
	rm -f $(BINARY)

dev:
	go run ./cmd/bot
