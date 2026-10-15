# Synthetic consumed-field fixtures

These JSON files are used by BOTH the actual Go handler contract test and the
Node real-provider decoder test. They are not full response captures, live data,
an API schema version, runtime mocks or an operational configuration. Object
fields outside this selected subset may be added without invalidating the Go
test; array resources/order and consumed values must match.

Fixed clock:2026-01-01T12:00:00.123456789Z. All IDs/aliases are explicitly DEMO.
No node address, credential or real IP is included; coordinates are the existing
country anchors. Active intent deliberately points to a revoked synthetic exit
so neither side can turn desired state into payload-health proof.

Changing a producer field requires updating/reviewing the shared fixture and
running BOTH Go and Node tests on the same SHA. Local absence of Go does not
constitute producer acceptance; exact-source CI evidence is required.
