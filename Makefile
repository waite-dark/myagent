.PHONY: build run clean test lint

APP_NAME := myclaw
BIN_DIR  := bin

build:
	go build -o $(BIN_DIR)/$(APP_NAME) .

run:
	go run .

clean:
	rm -rf $(BIN_DIR)

test:
	go test ./...

lint:
	go vet ./...
