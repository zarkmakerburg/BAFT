export type Language = 'fa' | 'en';
export type Theme = 'dark' | 'light';
export type View = 'overview' | 'nodes' | 'tunnels';
export interface Preferences { readonly language: Language; readonly theme: Theme }
export const PREFERENCE_KEY = 'baft.aurora.demo.preferences.v1';
export function parsePreferences(value: string | null, dark = true): Preferences {
  const fallback: Preferences = { language: 'fa', theme: dark ? 'dark' : 'light' };
  if (value === null) return fallback;
  try {
    const parsed: unknown = JSON.parse(value);
    if (typeof parsed !== 'object' || parsed === null || !('language' in parsed) || !('theme' in parsed)) return fallback;
    return { language: parsed.language === 'en' ? 'en' : 'fa', theme: parsed.theme === 'light' ? 'light' : 'dark' };
  } catch { return fallback; }
}
export function readPreferences(storage: Pick<Storage, 'getItem'>, dark = true): Preferences {
  try { return parsePreferences(storage.getItem(PREFERENCE_KEY), dark); } catch { return parsePreferences(null, dark); }
}
export function savePreferences(storage: Pick<Storage, 'setItem'>, preferences: Preferences): boolean {
  try { storage.setItem(PREFERENCE_KEY, JSON.stringify(preferences)); return true; } catch { return false; }
}
export function viewFromHash(hash: string): View {
  return hash === '#nodes' ? 'nodes' : hash === '#tunnels' ? 'tunnels' : 'overview';
}
