export interface SessionCredentials {
  sessionID: string;
  token: string;
}

const sessionIDPattern = /^[A-Za-z0-9_-]{22}$/;
const tokenPattern = /^[A-Za-z0-9_-]{43}$/;

export function sessionIDFromPath(pathname: string): string | null {
  const match = /^\/s\/([^/]+)\/?$/.exec(pathname);
  if (!match || !sessionIDPattern.test(match[1])) {
    return null;
  }
  return match[1];
}

export function credentialStorageKey(sessionID: string): string {
  return `rome.session.${sessionID}.token`;
}

export function loadCredentials(
  location: Pick<Location, "pathname" | "search" | "hash">,
  storage: Pick<Storage, "getItem" | "setItem" | "removeItem">,
  replaceURL: (url: string) => void,
): SessionCredentials | null {
  const sessionID = sessionIDFromPath(location.pathname);
  if (!sessionID) {
    return null;
  }

  const key = credentialStorageKey(sessionID);
  const fragment = new URLSearchParams(location.hash.startsWith("#") ? location.hash.slice(1) : location.hash);
  const fragmentToken = fragment.get("token");
  if (fragmentToken !== null) {
    replaceURL(location.pathname + location.search);
    try {
      storage.removeItem(key);
    } catch {
      // The live page can still use a valid fragment when storage is disabled.
    }
    if (!tokenPattern.test(fragmentToken)) {
      return null;
    }
    try {
      storage.setItem(key, fragmentToken);
    } catch {
      // The live page can still use the fragment token when storage is disabled.
    }
    return { sessionID, token: fragmentToken };
  }

  let storedToken: string | null = null;
  try {
    storedToken = storage.getItem(key);
  } catch {
    return null;
  }
  if (!storedToken || !tokenPattern.test(storedToken)) {
    return null;
  }
  return { sessionID, token: storedToken };
}

export function clearCredential(storage: Pick<Storage, "removeItem">, sessionID: string): void {
  try {
    storage.removeItem(credentialStorageKey(sessionID));
  } catch {
    // Storage may be unavailable in a hardened browser context.
  }
}
