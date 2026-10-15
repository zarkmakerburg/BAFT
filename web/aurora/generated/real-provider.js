import { ContractError, decodeOverview, decodeTunnels } from './validation.js';
export class ProviderError extends Error {
    code;
    constructor(code) {
        super(`Aurora request: ${code}`);
        this.code = code;
        this.name = 'ProviderError';
    }
}
export const RESPONSE_MAX_BYTES = 1_048_576;
/** Explicit construction only; imports never fetch or inspect a login/bootstrap. */
export function createRealProvider(options) {
    let origin;
    try {
        origin = new URL(options.origin);
    }
    catch {
        throw new ProviderError('configuration');
    }
    if (!['https:', 'http:'].includes(origin.protocol) || origin.origin !== options.origin ||
        !/^\/(?:[A-Za-z0-9_-]+\/)*$/.test(options.basePath) || typeof options.onSessionExpired !== 'function') {
        throw new ProviderError('configuration');
    }
    const timeoutMs = options.timeoutMs ?? 10_000;
    if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 10 || timeoutMs > 30_000)
        throw new ProviderError('configuration');
    const transport = options.fetcher ?? globalThis.fetch.bind(globalThis);
    const now = options.now ?? Date.now;
    const base = new URL(options.basePath, origin);
    async function request(path, decode, external) {
        if (external?.aborted)
            throw new ProviderError('aborted');
        const controller = new AbortController();
        let abortCode = 'aborted';
        const abortExternal = () => { abortCode = 'aborted'; controller.abort(); };
        external?.addEventListener('abort', abortExternal, { once: true });
        let rejectAbort = () => { };
        const aborted = new Promise((_, reject) => { rejectAbort = reject; });
        const onAbort = () => { rejectAbort(new ProviderError(abortCode)); };
        controller.signal.addEventListener('abort', onAbort, { once: true });
        const timer = setTimeout(() => { abortCode = 'timeout'; controller.abort(); }, timeoutMs);
        const url = new URL(`api/${path}`, base);
        const read = async () => {
            const response = await transport(url.href, {
                method: 'GET', credentials: 'same-origin', redirect: 'error', cache: 'no-store',
                headers: { Accept: 'application/json' }, signal: controller.signal
            });
            try {
                if (controller.signal.aborted)
                    throw new ProviderError(abortCode);
                if (response.redirected || (response.url !== '' && response.url !== url.href))
                    throw new ProviderError('http');
                if (response.status === 401) {
                    try {
                        options.onSessionExpired();
                    }
                    finally {
                        throw new ProviderError('session_expired');
                    }
                }
                if (!response.ok)
                    throw new ProviderError('http');
                if (!/^application\/json(?:\s*;|\s*$)/i.test(response.headers.get('content-type') ?? ''))
                    throw new ProviderError('invalid_data');
                const length = response.headers.get('content-length');
                if (length !== null && (!/^[0-9]+$/.test(length) || Number(length) > RESPONSE_MAX_BYTES))
                    throw new ProviderError('too_large');
                if (!response.body)
                    throw new ProviderError('invalid_data');
                const reader = response.body.getReader();
                const chunks = [];
                let size = 0;
                try {
                    while (true) {
                        const part = await Promise.race([reader.read(), aborted]);
                        if (part.done)
                            break;
                        size += part.value.byteLength;
                        if (size > RESPONSE_MAX_BYTES)
                            throw new ProviderError('too_large');
                        chunks.push(part.value);
                    }
                    const bytes = new Uint8Array(size);
                    let offset = 0;
                    for (const chunk of chunks) {
                        bytes.set(chunk, offset);
                        offset += chunk.byteLength;
                    }
                    let input;
                    try {
                        input = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
                    }
                    catch {
                        throw new ProviderError('invalid_data');
                    }
                    return decode(input, now());
                }
                finally {
                    // Do not block completion on a misbehaving stream's cancel handler.
                    void reader.cancel().catch(() => { });
                    reader.releaseLock();
                }
            }
            finally {
                if (response.body && !response.body.locked)
                    void response.body.cancel().catch(() => { });
            }
        };
        try {
            return await Promise.race([read(), aborted]);
        }
        catch (error) {
            if (controller.signal.aborted)
                throw new ProviderError(abortCode);
            if (error instanceof ProviderError)
                throw error;
            if (error instanceof ContractError)
                throw new ProviderError('invalid_data');
            throw new ProviderError('network');
        }
        finally {
            clearTimeout(timer);
            external?.removeEventListener('abort', abortExternal);
            controller.signal.removeEventListener('abort', onAbort);
        }
    }
    return Object.freeze({
        mode: 'real',
        getOverview: (signal) => request('overview', (input, at) => decodeOverview(input, 'real', at), signal),
        getTunnels: (signal) => request('tunnels', decodeTunnels, signal)
    });
}
