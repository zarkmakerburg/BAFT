# Aurora bootstrap CI attempt evidence

Source: `3d5511cf7a1b25a047de7d7464522e42f53e5a85`. Draft [PR204](https://github.com/zarkmakerburg/BAFT/pull/204).

The original main CI [job116314453057](https://github.com/zarkmakerburg/BAFT/actions/runs/38702691143/job/116314453057) FAILED at the global race step: TestIRToIRNoiseMeshRoundTrip timed out reading its payload after5.03s (mesh_noise_test.go:79); integration package116.143s. No data race diagnostic was present. This is not the earlier BCC/Argon2 cumulative timeout. A nearby TLS handshake log alone does not establish the root cause. The source diff contains only two Aurora planning documents; that does not prove the failure is harmless or explain it.

Static analysis and14 other main jobs succeeded. A single targeted rerun of the failed job at the identical SHA was accepted; its outcome remains pending in this immutable snapshot. No code, test deadline, KDF, security setting or workflow was modified. No additional application slice was started while the claim is active.

The complete original decoded log is compressed losslessly in failed-job-116314453057.log.gz.b64. manifest.json records exact byte lengths and SHA256 of the original and compressed bytes. Local gzip decompression round-trip and three credential-pattern scans (private-key block/GitHub token patterns) passed. This is a limited audit, not a universal secret detector. GitHub's original job remains linked.

Decode with `base64 --decode failed-job-116314453057.log.gz.b64 | gzip -d > failed-job.log`, then compare `sha256sum failed-job.log` to manifest.json. The first oversized JSON export was refused after truncation; the replacement10 chunks were length-checked before concatenation. No truncated output was archived.

This branch contains evidence only, based on the exact source. Source/PR204 stays unchanged. Native two-node/candidate acceptance remains owner-deferred/open; no production operation, merge or release. A dirty, unrelated poster asset in another local checkout was preserved untouched. No local Go reproduction was run because the toolchain is unavailable.
