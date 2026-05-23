.PHONY: run test build docker-up docker-down lint clean

VERSION ?= v0.1.0
LDFLAGS = -ldflags="-X main.version=$(VERSION)"

run:
	go run $(LDFLAGS) cmd/api/main.go

test:
	go test -v -race -cover ./...

build:
	go build $(LDFLAGS) -o bin/api cmd/api/main.go

docker-up:
	docker compose -f deploy/docker-compose.yml up --build -d

docker-down:
	docker compose -f deploy/docker-compose.yml down

lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed, running basic formatting check..."; \
		go fmt ./...; \
		go vet ./...; \
	fi

clean:
	rm -rf bin/
