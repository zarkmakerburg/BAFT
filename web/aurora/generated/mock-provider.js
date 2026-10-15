import { ProviderError } from './real-provider.js';
import { decodeOverview, decodeTunnels } from './validation.js';
export const DEMO_EPOCH = Date.parse('2026-01-01T12:00:00Z');
/** Fixed synthetic identifiers, no real addresses, network, cookies or timers. */
export function createMockProvider(now = () => DEMO_EPOCH) {
    const stamp = new Date(DEMO_EPOCH).toISOString();
    let phase = 'active';
    const tunnels = () => [{
            id: 'demo-tunnel-1', instance_id: 'demo-slot', ir_node: 'demo-ir-1', ex_node: 'demo-ex-1',
            phase, port: 18443, updated_at: stamp
        }];
    function overview() {
        return {
            generated_at: stamp,
            nodes: [
                { id: 'demo-ir-1', alias: 'DEMO Iran', role: 'worker', status: 'up', last_seen: stamp, agent_seen: stamp, country: 'IR', latency_ms: 82, in_bps: 0, out_bps: null },
                { id: 'demo-ex-1', alias: 'DEMO Germany', role: 'foreign', status: 'unknown', country: 'DE', latency_ms: null, in_bps: null, out_bps: null }
            ],
            links: phase === 'active' ? [{ tunnel_id: 'demo-tunnel-1', ir_node: 'demo-ir-1', ex_node: 'demo-ex-1', phase, port: 18443 }] : []
        };
    }
    const check = (signal) => { if (signal?.aborted)
        throw new ProviderError('aborted'); };
    return Object.freeze({
        mode: 'demo', label: 'DEMO',
        async getOverview(signal) {
            check(signal);
            return decodeOverview(overview(), 'demo', now());
        },
        async getTunnels(signal) {
            check(signal);
            return decodeTunnels(tunnels(), now());
        },
        async setTunnelPhase(id, next) {
            if (id !== 'demo-tunnel-1' || (next !== 'active' && next !== 'rolled_back'))
                throw new ProviderError('invalid_data');
            phase = next;
            return decodeTunnels(tunnels(), now());
        }
    });
}
