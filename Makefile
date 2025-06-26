lint:
	golangci-lint run ./... --fix

test:
	go tool gotestsum -- -cover ./...

test_race:
	go tool gotestsum -- -cover -race ./...

test_short:
	go tool gotestsum -- -cover -short ./...

test_integration:
	go tool gotestsum -- -cover -run Integration ./...

test_example_project:
	cd golang/example_project && make test
