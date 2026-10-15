/** Presentation contracts: desired deployment state is never delivery evidence. */
export type Mode = 'demo' | 'real';
export type Freshness = 'fresh' | 'stale' | 'future' | 'unknown';
export type NodeStatus = 'up' | 'down' | 'unknown';
export type NodeRole = 'worker' | 'foreign' | 'unknown';

export interface Metric {
  readonly value: number | null;
  // Overview exposes no per-metric observation timestamp. Never borrow another one.
  readonly observedAt: null;
  readonly freshness: 'unknown';
}

export interface NodeSummary {
  readonly id: string;
  readonly alias: string;
  readonly role: NodeRole;
  readonly revoked: boolean;
  readonly reportedStatus: NodeStatus;
  readonly effectiveStatus: NodeStatus;
  readonly lastSeen: string | null;
  readonly reportFreshness: Freshness;
  readonly agentSeen: string | null;
  readonly agentFreshness: Freshness;
  readonly country: string | null;
  readonly location: { readonly lat: number; readonly lon: number; readonly source: 'country-anchor' } | null;
  readonly latencyMs: Metric;
  readonly inBps: Metric;
  readonly outBps: Metric;
}

export interface TopologyLink {
  readonly id: string;
  readonly irNode: string;
  readonly exNode: string;
  readonly desiredPhase: string;
  readonly port: number;
  readonly deliveryHealth: 'unknown';
}

export interface Overview {
  readonly mode: Mode;
  readonly generatedAt: string;
  readonly fetchedAt: string;
  readonly freshness: Freshness;
  readonly nodes: readonly NodeSummary[];
  readonly links: readonly TopologyLink[];
}

export interface TunnelSummary extends TopologyLink {
  readonly instanceId: string | null;
  readonly updatedAt: string;
  readonly freshness: Freshness;
}

/** No operation in this interface can mutate a live tunnel or node. */
export interface ReadProvider {
  readonly mode: Mode;
  getOverview(signal?: AbortSignal): Promise<Overview>;
  getTunnels(signal?: AbortSignal): Promise<readonly TunnelSummary[]>;
}

export const EVIDENCE_TTL_MS = 180_000;

export function freshness(at: string | null, now: number, ttl = EVIDENCE_TTL_MS): Freshness {
  if (at === null || !Number.isFinite(now) || !Number.isFinite(ttl) || ttl <= 0) return 'unknown';
  const stamp = Date.parse(at);
  if (!Number.isFinite(stamp)) return 'unknown';
  if (stamp > now) return 'future';
  return now - stamp >= ttl ? 'stale' : 'fresh';
}

/** JSON uint64 numbers outside the safe range cannot be recovered by rounding. */
export function exactCounter(value: unknown): string | null {
  if (typeof value === 'number') return Number.isSafeInteger(value) && value >= 0 ? String(value) : null;
  if (typeof value !== 'string' || !/^(0|[1-9][0-9]{0,19})$/.test(value)) return null;
  return BigInt(value) <= 18_446_744_073_709_551_615n ? value : null;
}

/** Repaint cached evidence without fetching or renewing its original age. */
export function reassessOverview(snapshot: Overview, now: number): Overview {
  const age = freshness(snapshot.generatedAt, now);
  return { ...snapshot, freshness: age, nodes: snapshot.nodes.map(node => {
    const report = age === 'fresh' ? freshness(node.lastSeen, now) : 'unknown';
    return { ...node, reportFreshness: report,
      agentFreshness: age === 'fresh' ? freshness(node.agentSeen, now) : 'unknown',
      effectiveStatus: report === 'fresh' && !node.revoked ? node.reportedStatus : 'unknown' };
  }) };
}
