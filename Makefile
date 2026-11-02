.DEFAULT_GOAL := test
.PHONY: tools tidy install format lint test

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

tidy:
	@go mod tidy -v

install:
	@go mod download

format:
	@golangci-lint run --fix ./...

lint:
	golangci-lint version
	golangci-lint run -v ./...

test:
	go test -race -v -shuffle on ./...

test/%:
	go vet ./$(@:test/%=%)
	go test -race -v -shuffle on ./$(@:test/%=%)
