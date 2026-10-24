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
