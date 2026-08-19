import { clearCredential, credentialStorageKey, loadCredentials, sessionIDFromPath } from "./config";

const sessionID = "0JQ_LI4hPzL3isfD5U8wKw";
const token = "jcuFWqzjZ0GJ469zo6tvp5kpKlptRfxGif6SWPiqzPM";

describe("session credentials", () => {
  beforeEach(() => sessionStorage.clear());

  it("extracts a fragment token, stores it per tab, and removes the fragment", () => {
    const replaceURL = vi.fn();
    const credentials = loadCredentials(
      { pathname: `/s/${sessionID}`, search: "", hash: `#token=${token}` },
      sessionStorage,
      replaceURL,
    );

    expect(credentials).toEqual({ sessionID, token });
    expect(sessionStorage.getItem(credentialStorageKey(sessionID))).toBe(token);
    expect(replaceURL).toHaveBeenCalledWith(`/s/${sessionID}`);
  });

  it("reloads a token from session storage and clears it", () => {
    sessionStorage.setItem(credentialStorageKey(sessionID), token);
    expect(
      loadCredentials({ pathname: `/s/${sessionID}`, search: "", hash: "" }, sessionStorage, vi.fn()),
    ).toEqual({ sessionID, token });

    clearCredential(sessionStorage, sessionID);
    expect(sessionStorage.getItem(credentialStorageKey(sessionID))).toBeNull();
  });

  it("rejects invalid routes and tokens", () => {
    expect(sessionIDFromPath("/api/v1/sessions/id")).toBeNull();
    sessionStorage.setItem(credentialStorageKey(sessionID), token);
    expect(loadCredentials({ pathname: `/s/${sessionID}`, search: "", hash: "#token=short" }, sessionStorage, vi.fn())).toBeNull();
    expect(sessionStorage.getItem(credentialStorageKey(sessionID))).toBeNull();
    expect(loadCredentials({ pathname: "/s/not-an-id", search: "", hash: `#token=${token}` }, sessionStorage, vi.fn())).toBeNull();
  });
});
