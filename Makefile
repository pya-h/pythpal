BINARY=priceservice

.PHONY: build run test test-race clean

build:
	go build -o $(BINARY) ./cmd/server/

run: build
	./$(BINARY)

test:
	go test ./...

test-race:
	go test -race ./...

clean:
	rm -f $(BINARY)
