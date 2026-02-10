APP_NAME=google-workspace-github-sync

.PHONY: build test lint deploy dynamodb-up dynamodb-down dynamodb-setup dynamodb-reset

build:
	go build -o bin/$(APP_NAME) .

test:
	go test ./... -v

lint:
	golangci-lint run ./...

deploy:
	sam deploy --guided

# DynamoDB Local
dynamodb-up:
	docker compose up -d

dynamodb-down:
	docker compose down

dynamodb-reset:
	docker compose down -v
	./scripts/setup-local-dynamodb.sh

dynamodb-setup:
	./scripts/setup-local-dynamodb.sh
