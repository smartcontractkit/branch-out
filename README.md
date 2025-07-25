# Branch Out

A tool to accentuate the capabilities of [Trunk.io](https://trunk.io/)'s [flaky test tools](https://docs.trunk.io/flaky-tests/overview). When a test is detected as flaky, [Trunk.io sends a webhook](https://docs.trunk.io/flaky-tests/webhooks). From there, we can branch out to different services to customize the flaky test quarantine process.

See the [design doc](./design.md) for a more detailed look at how it works.

## Run

```sh
# See the help command for detailed instructions on how to run branch-out
branch-out --help
```

See [config.md](./config.md) for detailed config info.

## Contributing

We use [golangci-lint v2](https://golangci-lint.run/) for linting and formatting, and [pre-commit](https://pre-commit.com/) for pre-commit and pre-push checks.

```sh
pre-commit install # Install our pre-commit scripts
```

See the [Makefile](./Makefile) for helpful commands for local development.

```sh
make lint                 # Lint and format code
make watch                # Watch repo and run tests when .go files are saved
make test                 # Run all tests
make test_race            # Run all tests with race detection
make test_short           # Run all `short` tests
make test_integration     # Only run Integration tests
make test_example_project # Run example tests in the example_project directory
```
