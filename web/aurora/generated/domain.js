export const EVIDENCE_TTL_MS = 180_000;
export function freshness(at, now, ttl = EVIDENCE_TTL_MS) {
    if (at === null || !Number.isFinite(now) || !Number.isFinite(ttl) || ttl <= 0)
        return 'unknown';
    const stamp = Date.parse(at);
    if (!Number.isFinite(stamp))
        return 'unknown';
    if (stamp > now)
        return 'future';
    return now - stamp >= ttl ? 'stale' : 'fresh';
}
/** JSON uint64 numbers outside the safe range cannot be recovered by rounding. */
export function exactCounter(value) {
    if (typeof value === 'number')
        return Number.isSafeInteger(value) && value >= 0 ? String(value) : null;
    if (typeof value !== 'string' || !/^(0|[1-9][0-9]{0,19})$/.test(value))
        return null;
    return BigInt(value) <= 18446744073709551615n ? value : null;
}
/** Repaint cached evidence without fetching or renewing its original age. */
export function reassessOverview(snapshot, now) {
    const age = freshness(snapshot.generatedAt, now);
    return { ...snapshot, freshness: age, nodes: snapshot.nodes.map(node => {
            const report = age === 'fresh' ? freshness(node.lastSeen, now) : 'unknown';
            return { ...node, reportFreshness: report,
                agentFreshness: age === 'fresh' ? freshness(node.agentSeen, now) : 'unknown',
                effectiveStatus: report === 'fresh' && !node.revoked ? node.reportedStatus : 'unknown' };
        }) };
}
