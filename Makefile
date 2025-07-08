.PHONY: build generate lint test test_race test_short test_integration test_example_project

build:
	goreleaser build --snapshot --clean --single-target --single-target

generate:
	go generate ./...

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
