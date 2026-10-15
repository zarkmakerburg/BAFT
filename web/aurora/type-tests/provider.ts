import type { ReadProvider } from '../src/domain.js';
import type { DemoProvider } from '../src/mock-provider.js';
import { mountAurora } from '../src/shell.js';

export function checkBoundary(real: ReadProvider, demo: DemoProvider): void {
  // @ts-expect-error Live provider deliberately exposes no mutation capability.
  real.setTunnelPhase('demo-tunnel-1', 'active');
  // @ts-expect-error Real mode cannot be relabelled as demo.
  const pretend: DemoProvider = real;
  void pretend;
  // @ts-expect-error Demo action only accepts the explicitly modeled phases.
  void demo.setTunnelPhase('demo-tunnel-1', 'restart');
  // @ts-expect-error The isolated preview must not accept a live data provider.
  mountAurora(document.createElement('div'), real);
}
