# Example Project

An example Go project to test the effectiveness of our Go tools and analysis.

To run anything in this project, you need to use `-tags example_project`.

```sh
make test
```

Test results are determined by the `EXAMPLE_PROJECT_TESTS_MODE`, which can be:

* `pass`: All tests pass
* `fail`: All tests fail
* `skip`: All tests are skipped
* `mixed`: Each test randomly passes, skips, or fails

This is handy for getting test outputs.
