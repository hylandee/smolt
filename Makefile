.PHONY: build run test clean

build:
	go build -o bin/stronglifts ./cmd/stronglifts

run:
	go run ./cmd/stronglifts/main.go

test:
	go test -v ./...

test-auth:
	go test -v -run Auth ./internal/auth

clean:
	rm -rf bin/
	rm -f stronglifts.db

deps:
	go mod download
	go mod tidy

fmt:
	go fmt ./...

lint:
	golangci-lint run
