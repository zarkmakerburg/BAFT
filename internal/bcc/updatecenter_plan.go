package bcc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zarkmakerburg/baft/internal/release"
)

// Executors. The Update Center invents no update executor: an upgrade is the
// existing signed `update_baft` agent job; a rollback (and anything that job
// does not replace) is the verified installer, run by the operator on the node.
const (
	UpdateExecAgentJob  = "agent_job"
	UpdateExecInstaller = "installer"
	UpdateKindUpgrade   = "upgrade"
	UpdateKindRollback  = "rollback"
)

var (
	errUpdateUnknownNode    = errors.New("unknown node")
	errUpdateUnknownRelease = errors.New("unknown release")
	errUpdateBadRequest     = errors.New("invalid request")
)

// UpdateStep is one reviewed instruction. Code lets the UI show it in the
// operator's language; Text is the English fallback; Cmd is an exact command
// that is shown verbatim and never translated.
type UpdateStep struct {
	Code string `json:"code"`
	Text string `json:"text"`
	Cmd  string `json:"cmd,omitempty"`
}

type UpdatePlanApply struct {
	Endpoint     string `json:"endpoint"`
	Confirmation string `json:"confirmation"`
}

// UpdatePlan is deterministic and side-effect free. Hash binds node, kind,
// installed and target release identity, revocation sequence and every gate;
// apply recomputes it and refuses on any drift.
type UpdatePlan struct {
	Hash             string           `json:"hash"`
	OK               bool             `json:"ok"`
	NodeID           string           `json:"node_id"`
	Kind             string           `json:"kind"`
	FromVersion      string           `json:"from_version,omitempty"`
	FromRelease      string           `json:"from_release,omitempty"`
	ToVersion        string           `json:"to_version"`
	TargetDigest     string           `json:"target_digest"`
	Commit           string           `json:"commit"`
	RevocationSeq    uint64           `json:"revocation_sequence"`
	Executor         string           `json:"executor"`
	Gates            []PlanGate       `json:"gates"`
	Steps            []UpdateStep     `json:"steps"`
	Verify           []UpdateStep     `json:"verify"`
	Rollback         []UpdateStep     `json:"rollback"`
	Notes            []UpdateStep     `json:"notes"`
	InstallerPreview string           `json:"installer_preview,omitempty"`
	InstallerCommand string           `json:"installer_command,omitempty"`
	Apply            *UpdatePlanApply `json:"apply,omitempty"`
}

func (p UpdatePlan) computeHash() string {
	p.Hash = ""
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gate(name, status, detail string) PlanGate {
	return PlanGate{Name: name, Status: status, Detail: detail}
}

func buildUpdatePlan(ev updateEval, nodeID, kind, targetDigest string) (UpdatePlan, error) {
	var p UpdatePlan
	if !updateNodeIDRe.MatchString(nodeID) || !isHexSHA256(targetDigest) || (kind != UpdateKindUpgrade && kind != UpdateKindRollback) {
		return p, errUpdateBadRequest
	}
	var row *UpdateNodeRow
	for i := range ev.view.Nodes {
		if ev.view.Nodes[i].NodeID == nodeID {
			row = &ev.view.Nodes[i]
		}
	}
	if row == nil {
		return p, errUpdateUnknownNode
	}
	target, ok := ev.all[targetDigest]
	if !ok {
		return p, errUpdateUnknownRelease
	}
	p = UpdatePlan{
		NodeID: nodeID, Kind: kind, FromVersion: row.InstalledVersion, FromRelease: row.InstalledDigest,
		ToVersion: target.Version, TargetDigest: target.Digest, Commit: target.Commit,
		RevocationSeq: ev.view.RevocationSequence, Gates: []PlanGate{},
	}
	if kind == UpdateKindUpgrade {
		p.Executor = UpdateExecAgentJob
	} else {
		p.Executor = UpdateExecInstaller
	}
	fail := func(name, detail string) { p.Gates = append(p.Gates, gate(name, "FAIL", detail)) }
	pass := func(name, detail string) { p.Gates = append(p.Gates, gate(name, "PASS", detail)) }
	warn := func(name, detail string) { p.Gates = append(p.Gates, gate(name, "WARN", detail)) }

	if _, verified := ev.releases[targetDigest]; verified {
		pass("release signature", "manifest verifies against the pinned release root")
	} else {
		fail("release signature", "release does not verify against the pinned release root")
	}
	switch {
	case ev.revoked[target.SigningKeyID]:
		fail("release not revoked", "the release signing key is revoked")
	case ev.rev == nil:
		fail("release not revoked", "no revocation list has been imported")
	case ev.view.RevocationExpired:
		fail("release not revoked", "the revocation list expired; import a fresh bundle")
	default:
		pass("release not revoked", fmt.Sprintf("revocation list sequence %d", ev.view.RevocationSequence))
	}
	if row.NoticeCode == "node_revoked" {
		fail("server known and active", "access for this server is revoked")
	} else {
		pass("server known and active", "")
	}
	if row.InstalledVersion == "" {
		fail("installed version known", row.Notice)
	} else {
		pass("installed version known", row.InstalledVersion+" (reported digest matches a signed release)")
	}
	if row.InstalledVersion != "" {
		c, err := release.CompareVersions(target.Version, row.InstalledVersion)
		switch {
		case err != nil:
			fail("version order", err.Error())
		case kind == UpdateKindUpgrade && target.RollbackOnly:
			fail("version order", "imported as rollback-only; it cannot be an upgrade target")
		case kind == UpdateKindUpgrade && c > 0:
			pass("version order", row.InstalledVersion+" -> "+target.Version)
		case kind == UpdateKindUpgrade && c == 0:
			fail("version order", "already running this release")
		case kind == UpdateKindUpgrade:
			fail("version order", "this is a downgrade; use an explicit rollback plan")
		case c < 0:
			warn("version order", "explicit rollback "+row.InstalledVersion+" -> "+target.Version+"; the agent job refuses downgrades, so this runs through the installer with --allow-downgrade")
		default:
			fail("version order", "rollback target is not older than the installed release")
		}
	}
	need := []string{"baft"}
	if kind == UpdateKindUpgrade {
		need = []string{"baft", "baft-pair"}
	}
	present := map[string]bool{}
	for _, a := range target.Artifacts {
		present[a.Name] = true
	}
	p.Gates = append(p.Gates, artifactGate(ev, nodeID, need, present))

	p.OK = true
	for _, g := range p.Gates {
		if g.Status == "FAIL" {
			p.OK = false
		}
	}
	fillUpdateSteps(&p, target, row)
	if p.OK && p.Executor == UpdateExecAgentJob {
		p.Apply = &UpdatePlanApply{Endpoint: "/api/updates/apply", Confirmation: "UPDATE " + p.NodeID + " " + p.ToVersion}
	}
	p.Hash = p.computeHash()
	return p, nil
}

// artifactGate checks the signed manifest lists this node's artifacts. The node
// architecture comes only from the operator-confirmed platform evidence.
func artifactGate(ev updateEval, nodeID string, need []string, present map[string]bool) PlanGate {
	var plat NodePlatformEvidence
	for _, n := range ev.nodes {
		if n.ID == nodeID {
			plat = n.Platform
		}
	}
	if !plat.Confirmed || plat.Source != "operator-verified" {
		return gate("artifacts for this server", "FAIL", "operator-confirmed platform is missing")
	}
	for _, b := range need {
		name := fmt.Sprintf("%s-%s-%s", b, plat.OS, plat.Arch)
		if !present[name] {
			return gate("artifacts for this server", "FAIL", name+" is not in the signed manifest")
		}
	}
	return gate("artifacts for this server", "PASS", fmt.Sprintf("signed %s-%s artifacts present", plat.OS, plat.Arch))
}

func fillUpdateSteps(p *UpdatePlan, target UpdateRelease, row *UpdateNodeRow) {
	bundle := "baft-offline-" + target.Version
	p.Steps, p.Verify, p.Rollback, p.Notes = []UpdateStep{}, []UpdateStep{}, []UpdateStep{}, []UpdateStep{}
	st := func(c, t string) UpdateStep { return UpdateStep{Code: c, Text: t} }
	if p.Executor == UpdateExecAgentJob {
		p.Steps = append(p.Steps,
			st("agent_job_sign", "BCC signs one update_baft job for "+p.NodeID+" asking for "+p.ToVersion+". Nothing is pushed: the agent pulls the job."),
			st("agent_job_verify", "The agent downloads "+p.ToVersion+" from its own configured release base URL and verifies it with release.VerifyDir against its pinned release root, the current revocation list and its install trust state (no downgrade, no re-tag). A release that does not verify changes nothing."),
			st("agent_job_swap", "baft and baft-pair are replaced and the service is restarted; the previous binaries are kept as .prev. baft-agent and baft-bcc are not replaced by this job."),
			st("agent_job_health", "If the service is not active after the settle time, the agent restores the .prev binaries, restarts, reports the job as failed, and does not advance its trust state."),
		)
		p.Rollback = append(p.Rollback,
			st("rollback_auto", "Automatic: a failed start restores the previous binaries (no operator action)."),
			st("rollback_prev", "On the node, after a successful but unwanted update: the previous baft and baft-pair are next to the binaries as baft.prev and baft-pair.prev; the offline installer below restores the whole previous release with its own verification."),
			UpdateStep{Code: "rollback_installer", Text: "Offline installer rollback to " + p.FromVersion + " (needs that release's bundle):", Cmd: "sudo bash install.sh --offline . --allow-downgrade --yes"},
		)
		p.Notes = append(p.Notes, st("note_agent_config", "The agent must have release updates configured (release base URL and pinned release root). If it does not, the job fails harmlessly; use the installer path instead. Servers that cannot reach the release URL use the offline bundle."))
	} else {
		p.Steps = append(p.Steps,
			st("installer_obtain", "Get "+bundle+".tar.gz and its .sha256 (scripts/release/offline_bundle.sh output) through a channel you trust and compare the checksum."),
			UpdateStep{Code: "installer_extract", Text: "Unpack it on the server:", Cmd: "tar -xzf " + bundle + ".tar.gz && cd " + bundle},
		)
		p.InstallerPreview = "sudo bash install.sh --offline . --plan"
		p.InstallerCommand = "sudo bash install.sh --offline ."
		if p.Kind == UpdateKindRollback {
			p.InstallerPreview = "sudo bash install.sh --offline . --allow-downgrade --plan"
			p.InstallerCommand = "sudo bash install.sh --offline . --allow-downgrade --yes"
		}
		p.Steps = append(p.Steps,
			UpdateStep{Code: "installer_preview", Text: "Preview what it would change (changes nothing). Use the same role flags as the original install of this server (baft status shows them):", Cmd: p.InstallerPreview},
			UpdateStep{Code: "installer_run", Text: "Apply it. The installer verifies the release against the root pinned in install.sh and the bundled revocation list, keeps backups, checks the service stays up and restores the old files if not:", Cmd: p.InstallerCommand},
		)
		p.Rollback = append(p.Rollback,
			st("rollback_installer_auto", "The installer restores the previous files and service state itself if any step fails, and records the release only after the service is healthy."),
			st("rollback_forward", "If the rolled-back release is itself wrong, rerun the installer with the newest bundle (no --allow-downgrade): secrets, pairing and tunnels are kept."),
		)
		p.Notes = append(p.Notes, st("note_downgrade_gate", "The installer's own anti-downgrade state refuses an older release unless --allow-downgrade is given. BCC cannot lift that for you; this is deliberate."))
	}
	p.Verify = append(p.Verify,
		st("verify_report", "Within 15 minutes the node reports its binary digests. This row turns to current only when the reported baft digest equals the signed digest for "+p.ToVersion+"."),
		st("verify_health", "Optionally dispatch a health job to the node (runs baft doctor) before you proceed to the next server. Update servers one at a time."),
	)
	p.Notes = append(p.Notes,
		st("note_never_auto", "BCC never applies an update by itself: this plan only prepares reviewed steps."),
		st("note_bcc_self", "The Update Center does not update BCC itself. To upgrade baft-bcc, use the installer on the BCC host; a SQLite or backup schema change cannot be downgraded without a verified pre-upgrade snapshot (see KNOWN-LIMITATIONS)."),
	)
	if row.InstalledVersion == "" {
		p.Notes = append(p.Notes, st("note_unknown_installed", "The installed version is unknown, so this plan cannot be applied from BCC."))
	}
}
