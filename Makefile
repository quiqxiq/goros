.PHONY: test test-race test-integration

# Unit tests (default path, no network).
test:
	go test ./...

# Unit tests with the race detector (mandatory for gate/nativeapi per PLAN.md §15).
test-race:
	go test -race ./...

# Lab integration tests against real RouterOS devices (PLAN.md §15).
# Requires env: ROUTEROS_TEST_ADDRESS, ROUTEROS_TEST_USERNAME, ROUTEROS_TEST_PASSWORD
# (plus ROUTEROS_TEST_SSH_ADDRESS/USERNAME/PASSWORD for transport/ssh).
test-integration:
	go test -tags integration -count=1 -v ./...
