# Subprocess integration tests

Run `make test-integration` with the Go version in `go.mod`. The suite builds the
actual wrapper and a controllable fake server, then tests HTTP health, WebSocket
authentication and I/O, configuration, and process exits. It does not require
Docker, a Bedrock download, or EULA acceptance for a real server. Integration
tests use the `integration` build tag and run on every pull request.

The tests exercise the wrapper's subprocess protocol, not real Bedrock behavior.
They require a Unix-like host for process-group cleanup and use loopback TCP
ports. The fake is sent commands via the wrapper's WebSocket; it emits the
readiness marker only when instructed, so readiness is tested before and after
the transition.
