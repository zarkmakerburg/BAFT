# Managed ingress policy bootstrap

The local Linux installer can create the protected policy for an explicitly
managed node:

```sh
sudo baft-agent --provision-ingress-policy --node-id ir-lab-01
```

Use the node's actual enrolled identity. This local root-only command does not
contact BCC, read enrollment secrets, start polling, bind a port, change a
service, install a firewall rule or enable operational Apply. The sole path is
`/etc/baft/ingress/managed-node.json`; no CLI or signed job can supply another
policy path or service root list. Current fixed source roots are
`/etc/systemd/system` and `/usr/lib/systemd/system`, both pre-existing and
independently checked as protected root-owned directories.

Alpha's `ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, nodeID)` uses
Delta's `ingressinventory.MarshalInstalledManagedNodePolicyV1` and existing
`LoadInstalledManagedNodePolicy` wire contract. It creates a private `0700`
directory and a root-owned `0600` file using pinned no-follow directory FDs,
exclusive temporary creation, file/directory fsync and Linux no-replace atomic
rename. An identical installed identity is idempotent; a different node,
namespace, digest, unsafe owner/path or malformed existing file is refused.
Existing policy and foreign services are preserved on refusal.

An already held `PendingGuard` can call `BindInstalledPolicy(ctx)` to durably
pin the independently loaded profile's inode, device, SHA256, node, namespace
and versions into its lifecycle journal. `CheckInstalledPolicy(ctx)` requires
that exact pin under the same valid OS guard and pending claim. The pin survives
`MarkApplying` and process restart; recovery remains `NEEDS_RECONCILIATION`.
Replacing even identical bytes on a different inode refuses verification and
repinning, while retaining the original pending resource fence.

These APIs return inert identity data. They do not mint a live `Lease` or prove
complete inventory. The collector still refuses unproven service-manager,
runtime, container, namespace and durable-claim coverage; positive operational
Apply/Retire, receipt readiness and ready client config remain disabled.

Run the reproducible protected-filesystem LAB on Docker with Go installed:

```sh
./tests/e2e/managed_ingress_policy_lab.sh
```

The harness compiles for the Docker daemon's Linux architecture and runs in
scratch containers with networking disabled and no host filesystem mounts. It
tests protected provisioning, same/different-node concurrent no-replace,
non-root and malformed-policy refusal, journal restart/inode drift, service
preservation, installed inventory and descriptor cleanup, then executes the
actual CLI. The P0 ingress workflow runs this harness. No production policy is
created by ordinary tests: privileged bootstrap tests require both explicit LAB
environment opt-in and a sentinel in the disposable filesystem.

## Durable command correlation

A policy-pinned held guard can record `RecordCommandIntent(ctx, command)`.
This persists a canonical request, not signature authentication or permission.
The caller must separately verify the signed Agent job. Apply identity cannot
be replaced; Retire requires a distinct job ID for the same binding and an
existing Apply intent. Job IDs cannot be shared by independent operations.
Exact retries do not renew timestamps, leases or readiness. Commands survive
`MarkApplying` and restart; malformed or mismatched durable requests refuse
recovery while pending reservations remain fenced.

`ReservationStore.InspectCommandIntents(binding)` returns detached Apply/Retire
correlation data for recovery, including after policy drift. Reading an intent
never declares effects verified or releases resources. LiveLease admission is
still disabled until complete supported-environment coverage and real owned
listener/firewall/reconciliation contracts exist.

## Remaining source coverage boundary

A valid v1 policy identifies configuration and namespace; it does not prove a
closed service/runtime baseline. Source provenance now hashes the exact bytes
parsed and rechecks relevant files/directories after observation. This detects
sampled drift, including changes with restored mtimes, but cannot establish an
atomic host snapshot or exclude transient ABA/future changes. Alpha's private
directory lock does not serialize unrelated systemd, runtime or namespace
writers.

Before any live permit, the supported LAB must independently establish its
actual manager load paths/effective units, executable/configuration baseline,
process/cgroup and namespace visibility and relevant writer restrictions.
Retire additionally needs exact owned-listener distinction and host readback.
If installer-selected baseline/enforcement metadata is introduced, it needs a
versioned contract; no exclusion or supported boolean can replace measurement.
All presently unknown source classes and LiveLease denial remain in force.

## Agent startup provisioning (fail-closed)

On start, `baft-agent` builds the operational ingress executor only if all of
these hold: the installed managed node policy loads for this `--node-id`; the
`<state-dir>/ingress-authority` and `<state-dir>/ingress-journal` directories
are private (`0700`, owned by the agent user) and open; the journal chain
validates; and `/usr/sbin/nft` (or `/sbin/nft`) reports exactly one pre-provisioned
`inet baft ingress` chain (filter, input hook, priority -100, policy accept)
with the same handle on two reads. The agent never creates that table/chain.
Otherwise the executor stays nil: the capability header is not sent and v1 jobs
are BLOCKED. The LAB executor is never used. Leftover pending/APPLYING/
NEEDS_RECONCILIATION claims and journal entries are only logged and stay fenced.

## Installer: ingress chain and policy (`install.sh --agent-only`)

The agent never creates the chain, so the installer does. During
`install.sh --agent-only` it writes `/usr/local/lib/baft/ingress-chain.sh`
(the `baft-ingress-chain` helper, embedded in `install.sh`) and runs
`ensure`:

- If `inet baft ingress` is absent it creates `table inet baft` (a no-op if
  the table exists) and the empty chain
  `{ type filter hook input priority -100; policy accept; }` in one `nft -f -`
  transaction, then re-reads the ruleset and requires exactly the pinned layout
  that `ingressownership.ReadNFTLabChainHandle` accepts. It records ownership
  in `<agent-state-dir>/ingress-chain.owner` (`handle=`, `table_created=`).
- If the chain exists with the pinned layout it is left as is and is NOT
  claimed (no marker). If it exists with any other layout, or more than one
  matches, the helper exits 3, reports `REFUSED`, and changes nothing; the
  installer warns and continues, and the agent keeps the executor disabled.
- Foreign tables, chains and rules are never read-modified; `/etc/nftables.conf`
  is never edited. `--no-ingress-chain` skips all of this.
- Persistence: only when the marker exists, `baft-ingress-chain.service`
  (oneshot, `Before=baft-agent.service`, gated by `ConditionPathExists` on the
  marker) re-runs `ensure --boot` after a reboot. The agent unit is not
  modified. Without the marker the boot run is a no-op.
- `--provision-ingress-policy` (default OFF, `--agent-only` only) additionally
  runs `baft-agent --provision-ingress-policy --node-id <id>`. It never runs
  implicitly.

`baft uninstall` (agent in scope) runs `ingress-chain.sh remove` after a
successful uninstall: it deletes the chain only if the marker's handle and the
pinned layout match and the chain holds no rules (and deletes `table inet baft`
only if the installer created it and it is then empty), then disables and
removes the boot unit. Otherwise it leaves everything and reports why. If the
agent state directory was deleted first the marker is gone and the chain is
left in place.

Verification: `sudo tests/e2e/installer_ingress_chain.sh` (real nft in a private
netns) and `go test ./tests/docs`.

## Restart and boot recovery (v0.4.0)

### Tunnel transactions after a reboot or kill -9

`baft recover boot` (run by `baft-recover.service`, `DefaultDependencies=no`,
`Before=sysinit.target baft.service baft-agent.service`, ordered only, never
required) reconciles every instance (`tunnelnode.RecoverAllAtBoot`) from disk
evidence alone: `txn.json` + active marker + the SHA-256 of the live config,
unit and ownership marker against the digests the change recorded before it
switched each file and against its backups. Each file is `installed`,
`previous`, `foreign` or `missing`.

| Evidence | Action |
|---|---|
| prepared transaction | roll back (nothing live was switched; PKI restored, secrets dropped) |
| committed, any file foreign/missing, corrupt txn, bad/mismatched marker, several unfinished changes, rotation in progress | FENCED: nothing touched, exit status 1 |
| committed, config+unit+marker all `installed`, binary present, config validates | roll forward: kept committed and enabled; BCC still decides health/finalize |
| committed, otherwise (partial switch) | roll back from backups, byte for byte |
| marker claimed before any txn exists and the stage is empty | clear the claim |

Boot recovery never starts, stops or restarts a service (those verbs are
refused by its `System`), so it cannot deadlock against its own ordering.
Foreign units/configs are never read for modification. Session-level durable
recovery (`recovery.durable` in the config) is a different feature and stays
unsupported.

### Operational public ingress after an Agent restart

When the Agent dies every socket it owned is closed by the kernel. That is
proven from the kernel, never from an empty in-memory map. At startup, before
the poll loop and before any job can run, `Agent.ReconcileIngressAfterRestart`
(`baft-agent --ingress-reconcile=auto|report|off`, default `auto`;
`--reconcile-ingress` runs only this step and exits 1 if anything stays fenced)
handles each surviving reservation. Design choice: an automatic, remove-only
step that needs NO new signed job and mints NO lease, and only when ALL of
this holds (otherwise the operation stays fenced exactly as before):

1. Authority (`ingressauthority.RestartAuthority`): the held `PendingGuard`
   lock, pinned installed policy and a lifecycle in APPLYING_UNVERIFIED or
   NEEDS_RECONCILIATION that records a CONSUMED lease (`leased_jobs`) matching
   the stored intent and not in `superseded_jobs`. Retire wins over Apply. An
   admitted-but-never-leased intent, an expired unleased command or a bare
   journal entry authorizes nothing. Expiry is irrelevant to a remove-only
   action.
2. Sockets: complete protected coverage (`CollectInstalledManagedTCP`, no
   unknown source, no stopped service claim) and every `/proc/net/tcp` and
   `tcp6` row for the numeric port, in any state, contains no LISTEN and no
   socket with a nonzero inode owned by the agent uid. A foreign listener is
   reported (family, inode, uid) and never adopted or touched. Inode-less
   remnants (TIME_WAIT) and foreign non-listening rows are only reported.
   The inventory is repeated immediately before the delete.
3. Firewall: the pinned `inet baft ingress` chain (handle and base-chain
   layout) is read back; it holds either exactly the owner-tagged rule whose
   handle equals the journaled handle (a rule installed but not yet journaled
   is accepted only by exact owner tag + port + layout) or no rule with this
   port or tag. Foreign rule on the port, duplicate tag, handle mismatch or an
   unreadable ruleset deny.
4. Journal: phase and handle must be consistent with the rule (for example
   "retired" with a live rule, or a rule at phase `intent`, deny).

Then the rule is deleted by exact handle, absence is read back, the journal is
advanced `retire_intent` -> `retired` (an interrupted run is resumed from the
same evidence; an operation with no journaled rule goes straight to `retired`),
and the lifecycle state becomes NEEDS_RECONCILIATION. Nothing binds a socket
or adds a rule; `leased_jobs`, `command_intents` and `superseded_jobs` are never
written, so a spent job stays spent (a replayed Apply/Retire gets
`ErrLeaseReplay`; a new Retire after recovery is inert against a retired
journal; the ping-pong fences of the security review are untouched). The port
reservation stays pending (there is still no release API, same as after an
in-process Retire): ingress is down until BCC stages a new operation.

`--ingress-reconcile=report` evaluates the same evidence and writes nothing
(operator-visible dry run). Coverage is only complete in the supported
private-netns LAB profile, so on any other host the step fails closed.
BCC: for a finished Retire the Agent posts the existing `retired` receipt, for
a rolled-back Apply the existing `failed/reconciliation_required` receipt, via
the authenticated path. There is no "reconciled" receipt shape; BCC refuses
receipts outside the signed command's lifetime or conflicting with a stored
one, in which case BCC stays unaware (local state is already durable).

Residual gaps: uid (not cgroup) identifies "the agent's" sockets, which only
makes the check stricter; certificate rotations and the reservation release
are not recovered automatically.

Tests: `.github/workflows/operational-recovery.yml` (unit + real-kernel kill -9
tests in `tests/e2e/operational/recovery`, zero-SKIP verdict in
`tests/release/oprecovery`).
