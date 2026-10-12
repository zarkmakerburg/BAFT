# BCC default map location from IP

New nodes and existing nodes without a chosen country use an automatic location.
BCC looks up the public literal IP in the management address, then falls back
to `path_ipv4` and `path_ipv6`. No hostname is resolved. Private, loopback,
link-local, reserved, documentation and Tailscale/CGNAT addresses are skipped.

The country lookup runs in the background. It does not delay SSH host-key reads,
agent installation, enrollment or dashboard requests. The loop checks for new
nodes every 15 seconds and sends at most one lookup per second. Each request has
a three-second deadline; unsuccessful attempts retry after an hour. An IP edit
or a return to automatic allows a new attempt. Successful results persist across
restarts and appear through the existing overview/map polling.

By default BCC uses `https://api.country.is/<public-ip>`. Only the public IP is
sent; no credentials, node identifiers or BCC tokens are included. Set
`BAFT_BCC_GEOIP_URL` to a country.is-compatible HTTPS base URL to self-host, or
`off` to disable lookup. Redirects are rejected. Responses must match the
requested IP and contain a supported map country. A timeout, rate limit, invalid
response or unsupported country leaves the node enrolled with unknown location.
The operator can still choose a country manually.

The Servers page offers **Automatic from IP**, a manual country, and **No
location**. Manual choices, including explicitly hiding a location, survive
restarts and address edits. Existing country selections from older releases are
treated as manual. Choosing Automatic returns to IP lookup. Late lookup replies
cannot overwrite a manual choice, a revoked node or an edited IP.

API: `POST /api/nodes/location` accepts the existing `node_id` and `country`
fields, plus `automatic: true` with no country to restore automatic selection.
The endpoint retains admin/session/CSRF authorization. Country changes use the
durable audit outbox, including changes made by the background worker.

Nodes expose `country_source` (`ip` or `manual`) and the public `country_ip` used
for an inferred location. The overview includes `country_source` and
`location_approximate`. Positions remain the map's country-level anchors; both
manual countries and IP-derived countries are labelled **Approximate location**.
These coordinates are never presented as the physical server's verified address.
