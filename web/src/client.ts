import { authenticationPrefix, decodeControl, decodeMetadata, websocketProtocol, type SessionMetadata } from "./protocol";
import { retryDelay } from "./retry";

export type ClientState = "connecting" | "live" | "reconnecting" | "exited" | "error";

export interface ClientCallbacks {
  onState(state: ClientState, label: string): void;
  onMetadata(metadata: SessionMetadata): void;
  onReset(): void;
  onOutput(data: Uint8Array): void;
  onExit(code: number): void;
  onPermanentFailure(message: string, clearCredential: boolean): void;
}

export interface SessionClientOptions {
  sessionID: string;
  token: string;
  callbacks: ClientCallbacks;
  window?: Window;
  document?: Document;
  fetch?: typeof fetch;
  createWebSocket?: (url: string, protocols: string[]) => WebSocket;
}

export class SessionClient {
  private readonly sessionID: string;
  private token: string;
  private readonly callbacks: ClientCallbacks;
  private readonly browserWindow: Window;
  private readonly browserDocument: Document;
  private readonly request: typeof fetch;
  private readonly createWebSocket: (url: string, protocols: string[]) => WebSocket;

  private socket: WebSocket | null = null;
  private retryTimer: number | null = null;
  private retryAttempt = 0;
  private generation = 0;
  private connecting = false;
  private connectedOnce = false;
  private helloReceived = false;
  private stopped = false;
  private state: ClientState = "connecting";

  constructor(options: SessionClientOptions) {
    this.sessionID = options.sessionID;
    this.token = options.token;
    this.callbacks = options.callbacks;
    this.browserWindow = options.window ?? window;
    this.browserDocument = options.document ?? document;
    this.request = options.fetch ?? fetch.bind(globalThis);
    this.createWebSocket = options.createWebSocket ?? ((url, protocols) => new WebSocket(url, protocols));
  }

  start(): void {
    this.browserWindow.addEventListener("online", this.handleAvailabilityChange);
    this.browserWindow.addEventListener("offline", this.handleAvailabilityChange);
    this.browserDocument.addEventListener("visibilitychange", this.handleAvailabilityChange);
    this.connectNow();
  }

  dispose(): void {
    this.stopLifecycle();
    this.socket?.close(1000, "page closed");
    this.socket = null;
    this.token = "";
  }

  sendInput(data: Uint8Array<ArrayBuffer>): boolean {
    if (this.state !== "live" || this.socket?.readyState !== WebSocket.OPEN) {
      return false;
    }
    this.socket.send(data);
    return true;
  }

  sendResize(cols: number, rows: number): boolean {
    if (
      this.state !== "live" ||
      this.socket?.readyState !== WebSocket.OPEN ||
      !Number.isInteger(cols) ||
      !Number.isInteger(rows) ||
      cols < 1 ||
      rows < 1 ||
      cols > 65_535 ||
      rows > 65_535
    ) {
      return false;
    }
    this.socket.send(JSON.stringify({ type: "resize", cols, rows }));
    return true;
  }

  private connectNow(): void {
    if (this.stopped || this.connecting || this.socket) {
      return;
    }
    if (!this.browserWindow.navigator.onLine || this.browserDocument.hidden) {
      this.setState("reconnecting", this.browserDocument.hidden ? "Paused" : "Offline");
      return;
    }

    this.clearRetryTimer();
    this.connecting = true;
    const generation = ++this.generation;
    this.setState(this.connectedOnce ? "reconnecting" : "connecting", this.connectedOnce ? "Reconnecting" : "Connecting");
    void this.authenticate(generation);
  }

  private async authenticate(generation: number): Promise<void> {
    let response: Response;
    try {
      response = await this.request(`/api/v1/sessions/${encodeURIComponent(this.sessionID)}`, {
        headers: { Authorization: `Bearer ${this.token}` },
        cache: "no-store",
      });
    } catch {
      this.retryAfterFailure(generation);
      return;
    }
    if (!this.isCurrent(generation)) {
      return;
    }
    if (response.status === 401 || response.status === 403) {
      this.failPermanently("Authentication failed", true);
      return;
    }
    if (response.status === 404) {
      this.failPermanently("Session not found", true);
      return;
    }
    if (response.status === 429) {
      this.failPermanently("Too many authentication attempts", true);
      return;
    }
    if (!response.ok) {
      this.retryAfterFailure(generation);
      return;
    }

    let metadata: SessionMetadata | null = null;
    try {
      metadata = decodeMetadata(await response.json(), this.sessionID);
    } catch {
      // Handled as a permanent protocol failure below.
    }
    if (!metadata) {
      this.failPermanently("Invalid Session metadata", false);
      return;
    }
    this.callbacks.onMetadata(metadata);
    if (metadata.state === "exited") {
      this.finish(metadata.exitCode ?? 1);
      return;
    }
    this.openWebSocket(generation);
  }

  private openWebSocket(generation: number): void {
    const websocketURL = new URL(`/api/v1/sessions/${encodeURIComponent(this.sessionID)}/ws`, this.browserWindow.location.origin);
    websocketURL.protocol = websocketURL.protocol === "https:" ? "wss:" : "ws:";

    let socket: WebSocket;
    try {
      socket = this.createWebSocket(websocketURL.toString(), [websocketProtocol, authenticationPrefix + this.token]);
    } catch {
      this.retryAfterFailure(generation);
      return;
    }
    if (!this.isCurrent(generation)) {
      socket.close(1000, "superseded");
      return;
    }

    this.socket = socket;
    this.connecting = false;
    this.helloReceived = false;
    socket.binaryType = "arraybuffer";
    socket.addEventListener("open", () => {
      if (socket !== this.socket) {
        return;
      }
      if (socket.protocol !== websocketProtocol) {
        this.failPermanently("Server selected an incompatible protocol", false);
      }
    });
    socket.addEventListener("message", (event) => this.handleMessage(socket, event));
    socket.addEventListener("close", () => {
      if (socket !== this.socket) {
        return;
      }
      this.socket = null;
      this.connecting = false;
      if (!this.stopped) {
        this.scheduleReconnect();
      }
    });
  }

  private handleMessage(socket: WebSocket, event: MessageEvent): void {
    if (socket !== this.socket || this.stopped) {
      return;
    }
    if (event.data instanceof ArrayBuffer) {
      if (!this.helloReceived) {
        this.failPermanently("Terminal output arrived before hello", false);
        return;
      }
      this.callbacks.onOutput(new Uint8Array(event.data));
      return;
    }
    if (typeof event.data !== "string") {
      this.failPermanently("Unsupported server message", false);
      return;
    }

    const control = decodeControl(event.data, this.sessionID);
    if (!control) {
      this.failPermanently("Invalid server control message", false);
      return;
    }
    switch (control.type) {
      case "hello":
        if (this.helloReceived) {
          this.failPermanently("Duplicate hello message", false);
          return;
        }
        this.helloReceived = true;
        this.connectedOnce = true;
        this.retryAttempt = 0;
        this.callbacks.onReset();
        this.callbacks.onMetadata(control.session);
        this.setState("live", "Live");
        return;
      case "exit":
        this.finish(control.code);
        return;
      case "error":
        this.failPermanently(control.message || "Session transport error", false);
    }
  }

  private retryAfterFailure(generation: number): void {
    if (!this.isCurrent(generation)) {
      return;
    }
    this.connecting = false;
    this.scheduleReconnect();
  }

  private scheduleReconnect(): void {
    if (this.stopped) {
      return;
    }
    if (!this.browserWindow.navigator.onLine || this.browserDocument.hidden) {
      this.setState("reconnecting", this.browserDocument.hidden ? "Paused" : "Offline");
      return;
    }
    this.setState("reconnecting", "Reconnecting");
    const delay = retryDelay(this.retryAttempt++);
    this.clearRetryTimer();
    this.retryTimer = this.browserWindow.setTimeout(() => {
      this.retryTimer = null;
      this.connectNow();
    }, delay);
  }

  private finish(code: number): void {
    this.stopLifecycle();
    this.callbacks.onExit(code);
    this.setState("exited", `Exited ${code}`);
    this.socket?.close(1000, "session exited");
    this.socket = null;
    this.token = "";
  }

  private failPermanently(message: string, clearCredential: boolean): void {
    this.stopLifecycle();
    this.connecting = false;
    this.callbacks.onPermanentFailure(message, clearCredential);
    this.setState("error", "Error");
    this.socket?.close(1008, message.slice(0, 100));
    this.socket = null;
    this.token = "";
  }

  private setState(state: ClientState, label: string): void {
    this.state = state;
    this.callbacks.onState(state, label);
  }

  private clearRetryTimer(): void {
    if (this.retryTimer !== null) {
      this.browserWindow.clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  private stopLifecycle(): void {
    this.stopped = true;
    this.generation++;
    this.clearRetryTimer();
    this.browserWindow.removeEventListener("online", this.handleAvailabilityChange);
    this.browserWindow.removeEventListener("offline", this.handleAvailabilityChange);
    this.browserDocument.removeEventListener("visibilitychange", this.handleAvailabilityChange);
  }

  private isCurrent(generation: number): boolean {
    return !this.stopped && generation === this.generation;
  }

  private readonly handleAvailabilityChange = (): void => {
    if (this.stopped) {
      return;
    }
    if (!this.browserWindow.navigator.onLine || this.browserDocument.hidden) {
      this.clearRetryTimer();
      if (!this.socket) {
        this.setState("reconnecting", this.browserDocument.hidden ? "Paused" : "Offline");
      }
      return;
    }
    if (!this.socket && !this.connecting) {
      this.retryAttempt = 0;
      this.connectNow();
    }
  };
}
