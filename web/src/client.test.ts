import { SessionClient, type ClientCallbacks } from "./client";

const sessionID = "0JQ_LI4hPzL3isfD5U8wKw";
const token = "jcuFWqzjZ0GJ469zo6tvp5kpKlptRfxGif6SWPiqzPM";
const metadata = {
  id: sessionID,
  command: "bash",
  directory: "/tmp/project",
  state: "running",
  exitCode: null,
};

class FakeWebSocket extends EventTarget {
  static readonly OPEN = 1;
  readonly protocol = "rome.v1";
  readyState = 0;
  binaryType: BinaryType = "blob";
  readonly sent: Array<string | ArrayBufferLike | Blob | ArrayBufferView> = [];
  closeCode: number | null = null;

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
    this.sent.push(data);
  }

  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.dispatchEvent(new Event("open"));
  }

  message(data: string | ArrayBuffer): void {
    this.dispatchEvent(new MessageEvent("message", { data }));
  }

  close(code = 1000): void {
    this.closeCode = code;
    this.readyState = 3;
    this.dispatchEvent(new CloseEvent("close", { code }));
  }
}

function callbacks(): ClientCallbacks {
  return {
    onState: vi.fn(),
    onMetadata: vi.fn(),
    onReset: vi.fn(),
    onOutput: vi.fn(),
    onExit: vi.fn(),
    onPermanentFailure: vi.fn(),
  };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

async function flushPromises(): Promise<void> {
  for (let index = 0; index < 10; index++) {
    await Promise.resolve();
  }
}

describe("SessionClient", () => {
  beforeEach(() => {
    Object.defineProperty(document, "hidden", { configurable: true, value: false });
    Object.defineProperty(window.navigator, "onLine", { configurable: true, value: true });
  });

  it("authenticates, resets before output, sends binary input and resize, then exits", async () => {
    const socket = new FakeWebSocket();
    const events = callbacks();
    const request = vi.fn().mockResolvedValue(jsonResponse(metadata));
    const createWebSocket = vi.fn().mockReturnValue(socket);
    const client = new SessionClient({
      sessionID,
      token,
      callbacks: events,
      fetch: request,
      createWebSocket: createWebSocket as unknown as (url: string, protocols: string[]) => WebSocket,
    });

    client.start();
    await flushPromises();
    expect(request).toHaveBeenCalledWith(`/api/v1/sessions/${sessionID}`, expect.objectContaining({ cache: "no-store" }));
    expect(createWebSocket).toHaveBeenCalledWith(
      `ws://${window.location.host}/api/v1/sessions/${sessionID}/ws`,
      ["rome.v1", `rome.auth.${token}`],
    );

    socket.open();
    socket.message(JSON.stringify({ type: "hello", version: 1, session: metadata }));
    socket.message(Uint8Array.from([27, 91, 51, 50, 109]).buffer);
    expect(events.onReset).toHaveBeenCalledOnce();
    expect(events.onOutput).toHaveBeenCalledWith(Uint8Array.from([27, 91, 51, 50, 109]));
    expect(events.onState).toHaveBeenLastCalledWith("live", "Live");

    expect(client.sendInput(Uint8Array.from([104, 105, 10]))).toBe(true);
    expect(client.sendResize(100, 30)).toBe(true);
    expect(socket.sent[0]).toEqual(Uint8Array.from([104, 105, 10]));
    expect(socket.sent[1]).toBe('{"type":"resize","cols":100,"rows":30}');

    socket.message('{"type":"exit","code":7}');
    expect(events.onExit).toHaveBeenCalledWith(7);
    expect(events.onState).toHaveBeenLastCalledWith("exited", "Exited 7");
    expect(client.sendInput(Uint8Array.of(1))).toBe(false);
  });

  it("reconnects with bounded delay after an unexpected close", async () => {
    vi.useFakeTimers();
    const firstSocket = new FakeWebSocket();
    const secondSocket = new FakeWebSocket();
    const events = callbacks();
    const createWebSocket = vi.fn().mockReturnValueOnce(firstSocket).mockReturnValueOnce(secondSocket);
    const client = new SessionClient({
      sessionID,
      token,
      callbacks: events,
      fetch: vi.fn().mockImplementation(() => Promise.resolve(jsonResponse(metadata))),
      createWebSocket: createWebSocket as unknown as (url: string, protocols: string[]) => WebSocket,
    });

    client.start();
    await flushPromises();
    firstSocket.open();
    firstSocket.message(JSON.stringify({ type: "hello", version: 1, session: metadata }));
    firstSocket.close();
    expect(events.onState).toHaveBeenLastCalledWith("reconnecting", "Reconnecting");

    await vi.advanceTimersByTimeAsync(249);
    expect(createWebSocket).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    await flushPromises();
    expect(createWebSocket).toHaveBeenCalledTimes(2);

    secondSocket.open();
    secondSocket.message(JSON.stringify({ type: "hello", version: 1, session: metadata }));
    expect(events.onReset).toHaveBeenCalledTimes(2);
    client.dispose();
    vi.useRealTimers();
  });

  it.each([
    { status: 401, message: "Authentication failed" },
    { status: 404, message: "Session not found" },
    { status: 429, message: "Too many authentication attempts" },
  ])("stops and requests credential removal after HTTP $status", async ({ status, message }) => {
    const events = callbacks();
    const client = new SessionClient({
      sessionID,
      token,
      callbacks: events,
      fetch: vi.fn().mockResolvedValue(jsonResponse({}, status)),
      createWebSocket: vi.fn() as unknown as (url: string, protocols: string[]) => WebSocket,
    });

    client.start();
    await flushPromises();
    expect(events.onPermanentFailure).toHaveBeenCalledWith(message, true);
    expect(events.onState).toHaveBeenLastCalledWith("error", "Error");
  });

  it("pauses reconnect while offline and resumes on the online event", async () => {
    Object.defineProperty(window.navigator, "onLine", { configurable: true, value: false });
    const events = callbacks();
    const createWebSocket = vi.fn().mockReturnValue(new FakeWebSocket());
    const client = new SessionClient({
      sessionID,
      token,
      callbacks: events,
      fetch: vi.fn().mockResolvedValue(jsonResponse(metadata)),
      createWebSocket: createWebSocket as unknown as (url: string, protocols: string[]) => WebSocket,
    });

    client.start();
    expect(events.onState).toHaveBeenLastCalledWith("reconnecting", "Offline");
    expect(createWebSocket).not.toHaveBeenCalled();

    Object.defineProperty(window.navigator, "onLine", { configurable: true, value: true });
    window.dispatchEvent(new Event("online"));
    await flushPromises();
    expect(createWebSocket).toHaveBeenCalledOnce();
    client.dispose();
  });

  it("waits while hidden and connects when the page becomes visible", async () => {
    Object.defineProperty(document, "hidden", { configurable: true, value: true });
    const events = callbacks();
    const createWebSocket = vi.fn().mockReturnValue(new FakeWebSocket());
    const client = new SessionClient({
      sessionID,
      token,
      callbacks: events,
      fetch: vi.fn().mockResolvedValue(jsonResponse(metadata)),
      createWebSocket: createWebSocket as unknown as (url: string, protocols: string[]) => WebSocket,
    });

    client.start();
    expect(events.onState).toHaveBeenLastCalledWith("reconnecting", "Paused");
    expect(createWebSocket).not.toHaveBeenCalled();

    Object.defineProperty(document, "hidden", { configurable: true, value: false });
    document.dispatchEvent(new Event("visibilitychange"));
    await flushPromises();
    expect(createWebSocket).toHaveBeenCalledOnce();
    client.dispose();
  });
});
