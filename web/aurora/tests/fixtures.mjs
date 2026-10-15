export const epoch = Date.parse('2026-01-01T12:00:00Z');
export const stamp = new Date(epoch).toISOString();
export const overview = () => ({
  generated_at: stamp,
  nodes: [
    { id: 'ir-1', alias: 'Iran', role: 'worker', status: 'up', last_seen: stamp, agent_seen: stamp, country: 'IR', in_bps: 0, out_bps: null, latency_ms: null },
    { id: 'ex-1', alias: 'Germany', role: 'foreign', status: 'unknown', country: 'DE', lat: 51, lon: 9 }
  ],
  links: [{ tunnel_id: 't-1', ir_node: 'ir-1', ex_node: 'ex-1', phase: 'active', port: 8443 }]
});
export const tunnels = () => [{ id: 't-1', ir_node: 'ir-1', ex_node: 'ex-1', instance_id: 'slot-1', phase: 'active', port: 8443, updated_at: stamp }];
export const jsonResponse = (data, init = {}) => new Response(JSON.stringify(data), { headers: { 'content-type': 'application/json' }, ...init });
