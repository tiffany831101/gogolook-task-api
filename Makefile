.PHONY: run build test cover bench docker-build docker-run

IMAGE ?= task-api

run:
	go run ./cmd/server

build:
	go build -o bin/task-api ./cmd/server

# -race detects unsynchronised access to shared state (e.g. the task map).
test:
	go test -race -count=1 ./...

cover:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

# -run='^$$' skips unit tests so only benchmarks run.
bench:
	go test -run='^$$' -bench=. -benchmem ./...

docker-build:
	docker build -t $(IMAGE) .

docker-run: docker-build
	docker run --rm -p 8080:8080 $(IMAGE)
