# Legacy monitoring and overview freshness

This bounded Core/API change preserves the current JSON fields and lowercase
`up` / `down` / `unknown` vocabulary. It does not mount Aurora, change the
layered-health state machine, alter alert policy or promote a release candidate.

An observation is current only when its timestamp is nonzero, no later than the
query time, and strictly newer than `now - staleAfter`. The existing default
window is three minutes when the caller supplies a nonpositive duration.
The exact expiry boundary is stale. Future timestamps are uncertainty, not
evidence of health. No clock-skew correction is fabricated.

`MonitoringNode.status` is a legacy node-online summary. A recent authenticated
agent poll or successful BCC TCP probe still supplies independent online
evidence, even when telemetry is old or absent. This does **not** prove the
BAFT process, tunnel, handshake or application data path healthy. A current
failed TCP probe remains a negative reachability observation; a fresh agent
poll can still prove management reachability as in the prior contract.

Without independent current evidence, missing/zero/stale/future telemetry
produces `unknown`, not `down` or `up`. Expired or future telemetry cannot
overwrite a current independent probe/poll. Reported routes are projected as
`unknown` when their containing report is not current. Current reports still
lack individual route observation timestamps; this slice does not establish
the age or end-to-end validity of each probe within them.

`health_check_status` reflects only a current dated TCP probe. Unavailable
probe latency uses the existing `-1` sentinel. Overview uses that validated
probe result, keeping its JSON latency null when no current successful probe
exists, including when a recent agent independently proves the node online.
Current per-node **and aggregate** rate counters reject future observations.
Unknown/missing/future agent timestamps count as not current in AgentsStale.

LastSeen and cumulative counters retain their historical values. Read
projections do not rewrite accounting/history/cursors, route states or node
identity; clients must interpret the timestamp and status before calling a
counter current. Historical charts and uptime calculations are not redesigned.
The independent layered-health sampler/alert engine remains a separate review
surface and may have different stale-input semantics; this patch does not claim
to resolve all health views.

Regression evidence uses fixed timestamps, the exact expiry boundary,
zero/future/missing evidence, current independent signals, route-copy ownership,
real httptest monitoring/auth responses and the Command Center Overview model.
An additional failing intermediate regression caught the aggregate-rate path
after the per-node path was fixed; that failure is retained with the baseline.
Existing stale-node assertions change from DOWN to UNKNOWN without evidence,
or to UP when an actual fresh independent TCP probe proves reachability.

No auth/session/CSRF/write-handler, persistence, dependency, transport/shaping,
exit pool, restart/standby, installer, signing or operational change. Tests use
loopback fixtures and labelled dummy tokens; no real operational address or
secret is added. Browser/rendered parity and native Iran–Germany testing are
separate gates; the latter remains owner-deferred. An isolated draft and exact
source CI are required before any future integration decision by HQ.
