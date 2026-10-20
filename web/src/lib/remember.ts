// Choices remembered in this browser, if it lets us (a private window may not).

export function remembered<T extends string>(key: string, def: T, ok: readonly T[]): T {
  try {
    const v = localStorage.getItem("cairn." + key) as T | null;
    return v && ok.includes(v) ? v : def;
  } catch {
    return def;
  }
}

export function remember(key: string, v: string) {
  try {
    localStorage.setItem("cairn." + key, v);
  } catch {
    /* not kept */
  }
}
