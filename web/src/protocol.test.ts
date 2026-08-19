import { decodeControl, decodeMetadata } from "./protocol";

const sessionID = "0JQ_LI4hPzL3isfD5U8wKw";
const metadata = {
  id: sessionID,
  command: "bash",
  directory: "/tmp/project",
  state: "running",
  exitCode: null,
};

describe("protocol decoding", () => {
  it("decodes metadata and versioned controls", () => {
    expect(decodeMetadata(metadata, sessionID)).toEqual(metadata);
    expect(decodeControl(JSON.stringify({ type: "hello", version: 1, session: metadata }), sessionID)).toEqual({
      type: "hello",
      version: 1,
      session: metadata,
    });
    expect(decodeControl('{"type":"exit","code":130}', sessionID)).toEqual({ type: "exit", code: 130 });
    expect(decodeControl('{"type":"error","code":"slow_consumer","message":"too slow"}', sessionID)).toEqual({
      type: "error",
      code: "slow_consumer",
      message: "too slow",
    });
  });

  it("rejects malformed, mismatched, and unsupported controls", () => {
    expect(decodeMetadata({ ...metadata, id: "other" }, sessionID)).toBeNull();
    expect(decodeControl("not-json", sessionID)).toBeNull();
    expect(decodeControl(JSON.stringify({ type: "hello", version: 2, session: metadata }), sessionID)).toBeNull();
    expect(decodeControl('{"type":"unknown"}', sessionID)).toBeNull();
  });
});
