package ingressownership

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// NFTMutationRunner is injectable so the adapter can be tested without host
// privileges. Production admission still requires trusted chain provisioning
// and Linux kernel LAB acceptance.
type NFTMutationRunner interface {
	NFTRunner
	Run(context.Context, ...string) error
}

type NFTCommandRunner struct{ Binary string }

func (r NFTCommandRunner) ReadRuleset(ctx context.Context) ([]byte, error) {
	return NFTCommandReader{Binary: r.Binary}.ReadRuleset(ctx)
}

func (r NFTCommandRunner) Run(ctx context.Context, args ...string) error {
	if ctx == nil {
		return ErrFirewallUnverified
	}
	if r.Binary != "/usr/sbin/nft" && r.Binary != "/sbin/nft" {
		return ErrFirewallUnverified
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Restrict the command surface: callers must not turn this LAB runner
	// into a generic nft command executor.
	if !allowedNFTMutation(args) {
		return ErrFirewallUnverified
	}
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nft mutation failed: %w", err)
	}
	return nil
}

// NFTLabBackend may mutate only an already provisioned chain whose handle and
// base-chain layout are independently pinned by the caller. It never creates
// or flushes a table, chain, or unrelated rule.
type NFTLabBackend struct {
	mu                  sync.Mutex
	Runner              NFTMutationRunner
	ExpectedChainHandle uint64
}

func verifyNFTLabChain(raw []byte, expectedHandle uint64) error {
	if expectedHandle == 0 || len(raw) == 0 || len(raw) > 4<<20 {
		return ErrFirewallUnverified
	}
	var root struct {
		NFTables []json.RawMessage `json:"nftables"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&root); err != nil {
		return ErrFirewallUnverified
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return ErrFirewallUnverified
	}
	count := 0
	for _, item := range root.NFTables {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(item, &wrapper); err != nil {
			return ErrFirewallUnverified
		}
		chainRaw, ok := wrapper["chain"]
		if !ok {
			continue
		}
		var chain struct {
			Family string `json:"family"`
			Table  string `json:"table"`
			Name   string `json:"name"`
			Handle uint64 `json:"handle"`
			Type   string `json:"type"`
			Hook   string `json:"hook"`
			Prio   int    `json:"prio"`
			Policy string `json:"policy"`
		}
		if err := json.Unmarshal(chainRaw, &chain); err != nil {
			return ErrFirewallUnverified
		}
		if chain.Family == "inet" && chain.Table == "baft" && chain.Name == "ingress" {
			count++
			if chain.Handle != expectedHandle || chain.Type != "filter" || chain.Hook != "input" || chain.Prio != -100 || chain.Policy != "accept" {
				return ErrFirewallUnverified
			}
		}
	}
	if count != 1 {
		return ErrFirewallUnverified
	}
	return nil
}

func (b *NFTLabBackend) inspect(ctx context.Context) ([]FirewallObservation, error) {
	if ctx == nil {
		return nil, ErrFirewallUnverified
	}
	if b == nil || b.Runner == nil {
		return nil, ErrFirewallUnverified
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrFirewallUnverified, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := b.Runner.ReadRuleset(ctx)
	if err != nil || verifyNFTLabChain(raw, b.ExpectedChainHandle) != nil {
		return nil, errors.Join(ErrFirewallUnverified, err)
	}
	return ParseNFTIngressRules(raw)
}

// Inspect returns a fresh exact-owner observation without mutating policy.
func (b *NFTLabBackend) Inspect(ctx context.Context, plan RulePlan) (FirewallObservation, error) {
	if b == nil || ctx == nil {
		return FirewallObservation{}, ErrFirewallUnverified
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	observations, err := b.inspect(ctx)
	if err != nil {
		return FirewallObservation{}, err
	}
	return ReconcileRuleObservation(plan, observations)
}

// ObserveOwner is read-only. Behind the pinned chain handle and layout check
// it reports whether exactly this owner's rule is present (present=true) or
// whether nothing in the chain carries its port or its tag (present=false).
// Any other rule on the same dport, a duplicate tag, a mismatching family,
// table or chain, or an unparseable snapshot is an error, never "absent".
func (b *NFTLabBackend) ObserveOwner(ctx context.Context, plan RulePlan) (FirewallObservation, bool, error) {
	if b == nil || ctx == nil {
		return FirewallObservation{}, false, ErrFirewallUnverified
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	expected, err := PlanIngressRule(plan.Port, plan.Owner)
	if err != nil || expected != plan {
		return FirewallObservation{}, false, ErrInvalid
	}
	observations, err := b.inspect(ctx)
	if err != nil {
		return FirewallObservation{}, false, err
	}
	var match FirewallObservation
	count := 0
	for _, o := range observations {
		if o.Port != plan.Port && o.Tag != plan.Tag {
			continue
		}
		if VerifyRuleObservation(plan, o) != nil {
			return FirewallObservation{}, false, ErrFirewallUnverified
		}
		count++
		match = o
	}
	if count > 1 {
		return FirewallObservation{}, false, ErrFirewallUnverified
	}
	return match, count == 1, nil
}

// verifyAbsent is read-only. Absence alone never grants deletion authority;
// callers must independently retain evidence of a successful exact deletion.
func (b *NFTLabBackend) verifyAbsent(ctx context.Context, plan RulePlan, oldHandle uint64) error {
	if b == nil || ctx == nil || oldHandle == 0 {
		return ErrFirewallUnverified
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	expected, err := PlanIngressRule(plan.Port, plan.Owner)
	if err != nil || expected != plan {
		return ErrInvalid
	}
	observations, err := b.inspect(ctx)
	if err != nil {
		return err
	}
	for _, observation := range observations {
		if observation.Port == plan.Port || observation.Tag == plan.Tag || observation.Handle == oldHandle {
			return ErrFirewallUnverified
		}
	}
	return nil
}

// Install never removes a foreign rule after a partial command failure. A
// later retry must independently reobserve the exact owner rule.
func (b *NFTLabBackend) Install(ctx context.Context, plan RulePlan) (FirewallObservation, error) {
	if b == nil || ctx == nil {
		return FirewallObservation{}, ErrFirewallUnverified
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	expected, err := PlanIngressRule(plan.Port, plan.Owner)
	if err != nil || expected != plan {
		return FirewallObservation{}, ErrInvalid
	}
	before, err := b.inspect(ctx)
	if err != nil {
		return FirewallObservation{}, err
	}
	for _, observation := range before {
		if observation.Port == plan.Port || observation.Tag == plan.Tag {
			if err := VerifyRuleObservation(plan, observation); err != nil {
				return FirewallObservation{}, ErrConflict
			}
			return ReconcileRuleObservation(plan, before)
		}
	}
	if err := ctx.Err(); err != nil {
		return FirewallObservation{}, errors.Join(ErrFirewallUnverified, err)
	}
	if err := b.Runner.Run(ctx, "add", "rule", "inet", "baft", "ingress", "tcp", "dport", strconv.Itoa(int(plan.Port)), "accept", "comment", plan.Tag); err != nil {
		// The kernel may have accepted the command before its response failed.
		// Only after an attempted add with an independently empty preflight
		// may fresh exact ownership accompany the error for bounded rollback.
		// This never turns the failed operation into successful admission.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		after, inspectErr := b.inspect(cleanupCtx)
		if inspectErr != nil {
			return FirewallObservation{}, errors.Join(err, inspectErr)
		}
		observed, inspectErr := ReconcileRuleObservation(plan, after)
		if inspectErr != nil {
			return FirewallObservation{}, errors.Join(err, inspectErr)
		}
		return observed, err
	}
	after, err := b.inspect(ctx)
	if err != nil {
		return FirewallObservation{}, err
	}
	return ReconcileRuleObservation(plan, after)
}

// Remove rechecks exact owner, port, and kernel handle immediately before
// deletion. A handle swap or ambiguous snapshot refuses destructive cleanup.
func (b *NFTLabBackend) Remove(ctx context.Context, plan RulePlan, expectedHandle uint64) error {
	if b == nil || ctx == nil || expectedHandle == 0 {
		return ErrFirewallUnverified
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	expected, err := PlanIngressRule(plan.Port, plan.Owner)
	if err != nil || expected != plan {
		return ErrInvalid
	}
	for i := 0; i < 2; i++ {
		observations, err := b.inspect(ctx)
		if err != nil {
			return err
		}
		match, err := ReconcileRuleObservation(plan, observations)
		if err != nil || match.Handle != expectedHandle {
			return ErrFirewallUnverified
		}
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrFirewallUnverified, err)
	}
	if err := b.Runner.Run(ctx, "delete", "rule", "inet", "baft", "ingress", "handle", strconv.FormatUint(expectedHandle, 10)); err != nil {
		return err
	}
	after, err := b.inspect(ctx)
	if err != nil {
		return err
	}
	for _, observation := range after {
		if observation.Port == plan.Port || observation.Tag == plan.Tag {
			return ErrFirewallUnverified
		}
	}
	return nil
}

func allowedNFTMutation(args []string) bool {
	if len(args) == 11 && args[0] == "add" && args[1] == "rule" && args[2] == "inet" && args[3] == "baft" && args[4] == "ingress" && args[5] == "tcp" && args[6] == "dport" && args[8] == "accept" && args[9] == "comment" {
		port, err := strconv.ParseUint(args[7], 10, 16)
		return err == nil && port != 0 && port != 22 && strconv.FormatUint(port, 10) == args[7] && validNFTTag(args[10])
	}
	if len(args) == 7 && args[0] == "delete" && args[1] == "rule" && args[2] == "inet" && args[3] == "baft" && args[4] == "ingress" && args[5] == "handle" {
		handle, err := strconv.ParseUint(args[6], 10, 64)
		return err == nil && handle > 0 && strconv.FormatUint(handle, 10) == args[6]
	}
	return false
}
func validNFTTag(tag string) bool {
	if len(tag) != len("baft-ingress-")+32 || tag[:len("baft-ingress-")] != "baft-ingress-" {
		return false
	}
	for _, c := range tag[len("baft-ingress-"):] {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}
