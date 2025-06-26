test_fail:
	EXAMPLE_PROJECT_TESTS_MODE=fail go test -tags example_project -json ./... > all_failed.output

test_pass:
	EXAMPLE_PROJECT_TESTS_MODE=pass go test -tags example_project -json ./... > all_passed.output

test_skip:
	EXAMPLE_PROJECT_TESTS_MODE=skip go test -tags example_project -json ./... > all_skipped.output

test_mixed:
	EXAMPLE_PROJECT_TESTS_MODE=mixed go test -tags example_project -json ./... > mixed.output

test:
	-make test_fail
	-make test_mixed
	make test_pass
	make test_skip
