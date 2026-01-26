.PHONY: proto build run clean test lint help docker-up docker-down cluster-init

BINARY_NAME=flux
PROTO_DIR=internal/proto

help: ## Display help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

proto: ## Generate Go code using buf
	buf generate

build: ## Build binary
	go build -o bin/$(BINARY_NAME) cmd/server/main.go

test: ## Run tests
	go test -v -race ./...

docker-up: ## Start cluster
	docker-compose up --build -d

docker-down: ## Stop cluster
	docker-compose down

cluster-init: ## Join nodes to form a cluster
	@echo "Waiting for node-1 to stabilize..."
	@sleep 5
	@echo "Joining node-2..."
	docker exec flux-node-1 ./flux-server --join --id=node-2 --raft-addr=node-2:7000 --leader-addr=localhost:50051
	@echo "Joining node-3..."
	docker exec flux-node-1 ./flux-server --join --id=node-3 --raft-addr=node-3:7000 --leader-addr=localhost:50051