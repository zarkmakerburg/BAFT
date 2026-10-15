export const PREFERENCE_KEY = 'baft.aurora.demo.preferences.v1';
export function parsePreferences(value, dark = true) {
    const fallback = { language: 'fa', theme: dark ? 'dark' : 'light' };
    if (value === null)
        return fallback;
    try {
        const parsed = JSON.parse(value);
        if (typeof parsed !== 'object' || parsed === null || !('language' in parsed) || !('theme' in parsed))
            return fallback;
        return { language: parsed.language === 'en' ? 'en' : 'fa', theme: parsed.theme === 'light' ? 'light' : 'dark' };
    }
    catch {
        return fallback;
    }
}
export function readPreferences(storage, dark = true) {
    try {
        return parsePreferences(storage.getItem(PREFERENCE_KEY), dark);
    }
    catch {
        return parsePreferences(null, dark);
    }
}
export function savePreferences(storage, preferences) {
    try {
        storage.setItem(PREFERENCE_KEY, JSON.stringify(preferences));
        return true;
    }
    catch {
        return false;
    }
}
export function viewFromHash(hash) {
    return hash === '#nodes' ? 'nodes' : hash === '#tunnels' ? 'tunnels' : 'overview';
}
