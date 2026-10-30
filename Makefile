PHONY: tools
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

PHONY: tidy
tidy:
	@go mod tidy -v

PHONY: install
install:
	@go mod download

PHONY: format
format:
	@golangci-lint run --fix ./...

PHONY: lint
lint:
	golangci-lint version
	golangci-lint run -v ./...

PHONY: test
test:
	go test -race -v -shuffle on ./...

test/%:
	go vet ./$(@:test/%=%)
	go test -race -v -shuffle on ./$(@:test/%=%)
