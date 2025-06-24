lint:
	golangci-lint run ./... --fix

test:
	go tool gotestsum -- -cover ./...

test-race:
	go tool gotestsum -- -cover -race ./...
