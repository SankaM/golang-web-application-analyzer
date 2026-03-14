.PHONY: build run dev test coverage lint docker-build docker-run clean

BINARY      := bin/server
DOCKER_IMAGE := golang-web-analyzer

build:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/server

run: build
	./$(BINARY)

dev:
	go run ./cmd/server

test:
	go test ./... -v -race -count=1

coverage:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

lint:
	golangci-lint run ./...

docker-build:
	docker build -t $(DOCKER_IMAGE) .

docker-run:
	docker run --rm -p 8080:8080 $(DOCKER_IMAGE)

clean:
	rm -rf bin/ coverage.out coverage.html
