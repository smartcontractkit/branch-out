# Branch Out

A tool to accentuate the capabilities of [Trunk.io](https://trunk.io/)'s [flaky test tools](https://docs.trunk.io/flaky-tests/overview).

## Flow

1. Tests are run on develop
2. Results uploaded to Trunk
3. Trunk detects some flaky tests, marks them as quarantined and skips them with CI magic.
4. Trunk tells us they skipped a test
5. We branch out and create:
   1. GitHub PR to modify the code with `t.Skip()` calls.
   2. Jira Ticket to assign someone to fix the test and link it with Trunk's system
   3. (bonus) GitHub Issue to fix the flaky test. Ask GitHub Copilot for a PR attempt.

## Contributing

We use [golangci-lint v2](https://golangci-lint.run/) for linting and formatting, and [pre-commit](https://pre-commit.com/) for pre-commit and pre-push checks.

```sh
pre-commit install # Install our pre-commit scripts
```

See the [Makefile](./Makefile) for helpful commands for local development.

```sh
make build            # Build binaries, results placed in dist/
make lint             # Lint and format code
make bench            # Run all benchmarks

make test_short       # Run only short tests
make test_unit        # Run only unit tests
make test_integration # Run only integration tests
make test_full        # Run all tests with extensive coverage stats
make test_full_race   # Run all tests with extensive coverage stats and race detection
```

## // TODO:

* Properly receive Trunk webhooks
* Validate webhook calls
* Jira Ticket Creation + Linking
* Skip Golang Tests in PR
