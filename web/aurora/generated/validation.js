import { freshness, reassessOverview } from './domain.js';
export class ContractError extends Error {
    field;
    constructor(field) {
        super(`Invalid BCC data at ${field}.`);
        this.field = field;
        this.name = 'ContractError';
    }
}
function record(value, field) {
    if (typeof value !== 'object' || value === null || Array.isArray(value))
        throw new ContractError(field);
    return value;
}
function text(value, field, max = 256) {
    if (typeof value !== 'string' || value.length > max || /[\u0000-\u001f\u007f]/u.test(value))
        throw new ContractError(field);
    return value;
}
function id(value, field) {
    const out = text(value, field, 128);
    if (!/^[A-Za-z0-9][A-Za-z0-9._:-]*$/.test(out))
        throw new ContractError(field);
    return out;
}
function flag(value, field) {
    if (value === undefined)
        return false;
    if (typeof value !== 'boolean')
        throw new ContractError(field);
    return value;
}
function list(value, field, max) {
    if (!Array.isArray(value) || value.length > max)
        throw new ContractError(field);
    return value;
}
function nullableNumber(value, field, min = 0, max = Number.MAX_SAFE_INTEGER) {
    if (value === undefined || value === null)
        return null;
    if (typeof value !== 'number' || !Number.isFinite(value) || value < min || value > max)
        throw new ContractError(field);
    return value;
}
export function timestamp(value, field = 'timestamp') {
    const out = text(value, field, 40);
    const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$/.exec(out);
    if (!match || !Number.isFinite(Date.parse(out)))
        throw new ContractError(field);
    const year = Number(match[1]), month = Number(match[2]), day = Number(match[3]);
    const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
    const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1];
    if (!days || day < 1 || day > days || Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59)
        throw new ContractError(field);
    return out;
}
function optionalTime(value, field) {
    return value === undefined || value === null ? null : timestamp(value, field);
}
function metric(value, field) {
    return { value: nullableNumber(value, field), observedAt: null, freshness: 'unknown' };
}
function node(value, now, snapshotFresh) {
    const n = record(value, 'node');
    const role = text(n['role'], 'node.role');
    const status = text(n['status'], 'node.status');
    const reportedStatus = status === 'up' || status === 'down' ? status : 'unknown';
    const revoked = flag(n['revoked'], 'node.revoked') || status === 'revoked';
    const lastSeen = optionalTime(n['last_seen'], 'node.last_seen');
    const agentSeen = optionalTime(n['agent_seen'], 'node.agent_seen');
    const reportFreshness = freshness(lastSeen, now);
    const country = n['country'] === undefined || n['country'] === '' || n['country'] === null ? null : text(n['country'], 'node.country', 2);
    if (country !== null && !/^[A-Z]{2}$/.test(country))
        throw new ContractError('node.country');
    const lat = nullableNumber(n['lat'], 'node.lat', -90, 90);
    const lon = nullableNumber(n['lon'], 'node.lon', -180, 180);
    return {
        id: id(n['id'], 'node.id'), alias: text(n['alias'], 'node.alias'),
        role: role === 'worker' || role === 'foreign' ? role : 'unknown', revoked,
        reportedStatus, effectiveStatus: snapshotFresh && reportFreshness === 'fresh' && !revoked ? reportedStatus : 'unknown',
        lastSeen, reportFreshness, agentSeen, agentFreshness: freshness(agentSeen, now), country,
        location: lat !== null && lon !== null ? { lat, lon, source: 'country-anchor' } : null,
        latencyMs: metric(n['latency_ms'], 'node.latency_ms'),
        inBps: metric(n['in_bps'], 'node.in_bps'), outBps: metric(n['out_bps'], 'node.out_bps')
    };
}
function link(value, tunnel = false) {
    const t = record(value, 'tunnel');
    const port = nullableNumber(t['port'], 'tunnel.port', 1, 65535);
    if (port === null || !Number.isInteger(port))
        throw new ContractError('tunnel.port');
    return {
        id: id(t[tunnel ? 'id' : 'tunnel_id'], 'tunnel.id'),
        irNode: id(t['ir_node'], 'tunnel.ir_node'), exNode: id(t['ex_node'], 'tunnel.ex_node'),
        desiredPhase: text(t['phase'], 'tunnel.phase', 64), port, deliveryHealth: 'unknown'
    };
}
function unique(values, field) {
    if (new Set(values.map(v => v.id)).size !== values.length)
        throw new ContractError(field);
}
export function decodeOverview(value, mode, now) {
    if (!Number.isFinite(now))
        throw new ContractError('clock');
    const o = record(value, 'overview');
    const generatedAt = timestamp(o['generated_at'], 'overview.generated_at');
    const snapshotFreshness = freshness(generatedAt, now);
    const nodes = list(o['nodes'], 'overview.nodes', 2000).map(n => node(n, now, snapshotFreshness === 'fresh'));
    const links = list(o['links'], 'overview.links', 5000).map(l => link(l));
    unique(nodes, 'overview.nodes.duplicate');
    unique(links, 'overview.links.duplicate');
    const known = new Set(nodes.map(n => n.id));
    if (links.some(l => !known.has(l.irNode) || !known.has(l.exNode) || l.irNode === l.exNode))
        throw new ContractError('overview.links.scope');
    return reassessOverview({ mode, generatedAt, fetchedAt: new Date(now).toISOString(), freshness: snapshotFreshness, nodes, links }, now);
}
export function decodeTunnels(value, now) {
    const tunnels = list(value, 'tunnels', 5000).map(raw => {
        const t = record(raw, 'tunnel');
        const updatedAt = timestamp(t['updated_at'], 'tunnel.updated_at');
        const instanceId = t['instance_id'] === undefined || t['instance_id'] === '' ? null : id(t['instance_id'], 'tunnel.instance_id');
        const out = { ...link(t, true), instanceId, updatedAt, freshness: freshness(updatedAt, now) };
        if (out.irNode === out.exNode)
            throw new ContractError('tunnel.scope');
        return out;
    });
    unique(tunnels, 'tunnels.duplicate');
    return tunnels;
}
