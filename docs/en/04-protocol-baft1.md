# 04 — BAFT/1 protocol and state machine

BAFT/1 is a binary framing protocol used only inside an authenticated encrypted Carrier. TLS provides confidentiality and integrity; BAFT does not add a custom security checksum.

## Frame format

All binary integers are big-endian.

| Offset | Size | Field | Meaning |
|---:|---:|---|---|
| 0 | 4 | `frame_len` | total frame length including header |
| 4 | 1 | `type` | frame type |
| 5 | 1 | `flags` | must be zero in v1 |
| 6 | 2 | `reserved` | must be zero in v1 |
| 8 | 8 | `stream_id` | zero for Session control, non-zero for Flow |
| 16 | 8 | `offset` | type-specific byte/state offset |
| 24 | variable | `payload` | maximum 65536 bytes |

Header size is 24 bytes; total frame size is 24..65560. The parser validates bounds and type before payload allocation, detects offset overflow, and performs incremental reads without relying on TCP or HTTP/2 frame boundaries.

## Frame types

`HELLO`, `HELLO_ACK`, `OPEN`, `OPEN_OK`, `OPEN_ERR`, `DATA`, `ACK`, `WINDOW`, `FIN`, `FIN_ACK`, `RESET`, `RESUME_STATE`, `RESUME_DONE`, `READY`, `PING`, `PONG`, `GOAWAY`, `PROFILE_PROPOSE`, `PROFILE_ACCEPT`, `PROFILE_COMMIT`, and `PADDING` are reserved by BAFT/1.

Unknown types/flags, non-zero reserved bits, or invalid stream placement are protocol errors.

## Session startup

New Sessions use:

```text
HELLO → HELLO_ACK → two-sided READY → application frames
```

HELLO metadata never overrides the authenticated certificate identity.

### Capability negotiation (capability set v1)

`HELLO.capabilities` and the optional `HELLO_ACK.capabilities` carry what each node's configuration enables. The Session negotiates the **intersection**; nothing is ever downgraded silently.

| Token | Advertised when | Meaning |
|---|---|---|
| `carrier.h2` | `transport.primary` or `fallback` is `h2` | Carrier kind served (EX) or dialable (IR). |
| `carrier.ws` | `transport.primary` or `fallback` is `ws` | Same for WebSocket. Always implies `noise.v1`. |
| `noise.v1` | a `noise` section exists | The carrier is authenticated by the pinned-key Noise handshake. |
| `shape.v1` | `noise.record_shaping.enabled` | Record shaping is on; both peers must enable it. |

Not negotiated, because they are local behaviour with no peer-visible effect: the uTLS fingerprint profile, the per-Flow idle timeout. Multiplexing is not part of capability set v1.

Wire format. The list is canonical: at most 16 tokens, each `[a-z0-9][a-z0-9._-]{0,31}`, **strictly ascending** (duplicates, reordering, upper-case aliases, empty tokens and `null` are refused). Well-formed tokens this build does not know are ignored (never enabled), so a later release can add tokens without breaking this one.

Rules.

1. A node enables a capability only if both sides advertised it.
2. `transport.require_capabilities` (optional, default empty) lists capabilities the peer must negotiate. A peer lacking one is refused with a typed `*protocol.CapabilityError` (`required_missing`) and the carrier is closed. No `HELLO_ACK` is sent by a refusing EX. This also refuses a v0.3.7 peer, so enable it only after the upgrade is complete.
3. Independently, a node expects `noise.v1` and `shape.v1` whenever it has them enabled, and an IR expects each carrier kind it may dial. If the peer negotiates capabilities but lacks one, the Session is refused (`expected_missing`). `carrier.ws` without `noise.v1` is refused (`ws_requires_noise`). Noise is never optional where configuration requires it: the carrier itself already refuses a non-Noise peer, and the capability check is a second, explicit gate.
4. Refusal reasons are `required_missing`, `expected_missing`, `ws_requires_noise`, `unsolicited_capabilities` and `invalid`. They are logged, counted as handshake errors and exported as `baft_capability_refusals_total{reason}`.

Compatibility with v0.3.7. A v0.3.7 node sends `capabilities: []`; its HELLO validation only bounds the list at 16 entries and ignores tokens, but its `HELLO_ACK` decoder rejects unknown fields. Therefore a v0.4 IR may advertise tokens to any EX, and a v0.4 EX adds `capabilities` to `HELLO_ACK` **only** when the IR advertised at least one token. A peer that negotiates nothing is the *baseline*: the frames are byte-identical to v0.3.7, `expect` rules are waived for it, and only an explicit `require_capabilities` refuses it. Baseline sessions are visible as `baft_capability_baseline_sessions`; negotiated ones as `baft_capability_negotiated_sessions{capability}`.

Rolling-upgrade rule: upgrade both nodes with `require_capabilities` unset; set it only afterwards. A v0.3.7 config parser rejects unknown keys, so do not ship a `require_capabilities` config to a node that has not been upgraded.

## OPEN

OPEN carries a Route ID plus a random open nonce, never an arbitrary destination address. Duplicate OPEN with identical identity must be idempotent and must not redial the target.

## DATA / ACK / WINDOW

Each Flow direction has its own byte-offset space.

- DATA offset is the first byte position in that payload.
- ACK reports the next contiguous byte expected by the receiver.
- ACK means BAFT acceptance, not final target application processing.
- WINDOW is an absolute maximum acceptable offset, not a delta.
- duplicate or overlapping DATA is never written twice to the target.
- gaps and flow-control violations are rejected.

## FIN

FIN carries the final offset for one direction. The receiver verifies it matches accepted data, half-closes the destination socket when supported, and replies with FIN_ACK.

## RESET / fixed errors

Wire-visible errors use a fixed code vocabulary such as `AUTH_FAILED`, `FLOW_CONTROL_ERROR`, `ROUTE_DENIED`, `TARGET_UNREACHABLE`, `RESOURCE_EXHAUSTED`, `STALE_EPOCH`, and `PROTOCOL_ERROR`. Raw OS/file-path errors are not sent to the peer.

A Flow that ends before the peer's FIN was passed to the local socket (RESET in either direction, a failed OPEN, revocation, or the end of the Session without recovery) is closed abortively: the local socket gets `SO_LINGER=0`, so the application sees a connection reset instead of a clean EOF that would look like a complete stream. A Flow that ended through FIN keeps the graceful close.

## Resume frames

RESUME frame types are reserved by the protocol, but complete resume semantics belong to Stage D. Parsing a reserved type is not equivalent to implementing recovery.
