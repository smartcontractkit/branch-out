.PHONY: build watch watch_race lint test test_race test_short test_integration test_example_project generate-mocks

build:
	goreleaser build --snapshot --clean --single-target --single-target

lint:
	golangci-lint run ./... --fix

watch:
	go tool gotestsum --watch -- -cover ./...

watch_race:
	go tool gotestsum --watch -- -cover -race ./...

generate: ## Generate mocks for all interfaces
	go generate ./...

test:
	go tool gotestsum -- -cover ./...

test_coverage:
	go tool gotestsum -- -coverprofile=./coverage.out ./...
	-go tool go-test-coverage -config=./.testcoverage.yml -profile=./coverage.out
	go tool cover -html=coverage.out -o=coverage.html

test_race:
	go tool gotestsum -- -cover -race ./...

test_short:
	go tool gotestsum -- -cover -short ./...

test_integration:
	go tool gotestsum -- -cover -run Integration ./...

test_example_project:
	cd golang/example_project && make test
