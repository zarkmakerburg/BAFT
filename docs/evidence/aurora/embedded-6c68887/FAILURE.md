# Current-source full-CI failure — preserve before remediation

PR2146c68887a/source treec7c669cc: CI38783727212 finished14SUCCESS/1FAILURE. Test116590245451 failed Race detector, Vet/fuzz SKIPPED. Full3376211-character log preserved; no WARNING: DATA RACE observed. Actual failure is TestStaleFinalizeFailureCannotRegressNewIncarnation at tests/integration/recovery_runtime_test.go:2058, before stale failure injection. Snapshot shows Frozen:true despite ActivationComplete/ApplicationReady/TransactionStable:true and FINALIZED; source-level diagnosis in progress. This is not a detected Go memory race or a CSS failure. UI/static and separately combined80Node/Go success do not erase the failure. No rerun requested. Current test file is byte-identical across UI214 and Core215. No runtime/test source remediation or pass is claimed by this record.

2026-10-15T18:28:40.1410119Z 2026/10/15 18:22:06 baft listener startup: bind success configured=127.0.0.1:37793 actual=127.0.0.1:37793
2026-10-15T18:28:40.1410644Z 2026/10/15 18:22:06 baft listener startup: serve started actual=127.0.0.1:37793
2026-10-15T18:28:40.1411246Z --- FAIL: TestStaleFinalizeFailureCannotRegressNewIncarnation (0.14s)
2026-10-15T18:28:40.1411899Z     recovery_runtime_test.go:77: proxy accepted ProxyConnectionID=1
2026-10-15T18:28:40.1412408Z     recovery_runtime_test.go:63: proxy CutAll cohort=[1]
2026-10-15T18:28:40.1412895Z     recovery_runtime_test.go:77: proxy accepted ProxyConnectionID=2
2026-10-15T18:28:40.1417322Z     recovery_runtime_test.go:2058: new incarnation not stable before stale failure: {SessionID:76b957b23ce0cddba0fd7cbfe4a96714 Epoch:2 Owner:shard-0-replacement-1 CarrierGeneration:3 PreparedIncarnation:2 PreparedID:0xc0009d7d40 SenderID:796 SenderStopped:false PostCommitFailures:0 RecoveryAttempts:1 RecoverySignalPending:false CurrentCarrierUsable:true Frozen:true ActivationComplete:true ApplicationReady:true TransactionStable:true ReplayHighWatermark:19 ReplayPeerAccepted:19 ReplayOutstanding:false FinStable:true FinalizationStable:true TxnState:FINALIZED CandidateID:shard-0-replacement-1 NextEpoch:2 PlanDigest:74c2073835a59c45223aafe31f5d27fbd3374ea93d2dbf618c982fdb33b3f15b Flows:[{StreamID:3 ReplaySource:19 ReplayHighWatermark:19 ReplayOutstanding:false PeerAccepted:19 TxAcked:19 TxNext:19 FinSent:false FinAcked:false FinAckSent:false FinAckConfirmed:false}]}
2026-10-15T18:28:40.1420723Z     recovery_runtime_test.go:63: proxy CutAll cohort=[2]
2026-10-15T18:28:40.1421523Z 2026/10/15 18:22:06 baft shard 0 recovery attempt failed: recovery commit outcome uncertain: status carrier (h2): h2: OpenNoise failed phase=start elapsed=0ms trace=: transport error
2026-10-15T18:28:40.1422449Z 2026/10/15 18:22:06 baft listener startup: goroutine created configured=127.0.0.1:45569

