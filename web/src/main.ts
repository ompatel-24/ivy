import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import "./style.css";

import { SessionClient, type ClientState } from "./client";
import { clearCredential, loadCredentials } from "./config";
import { HelperControls } from "./helper-controls";
import type { SessionMetadata } from "./protocol";
import { SizeTracker } from "./size";

const terminalElement = requiredElement("terminal");
const commandElement = requiredElement("command");
const directoryElement = requiredElement("directory");
const statusElement = requiredElement("status");
const statusLabelElement = requiredElement("status-label");
const helperControlsElement = requiredElement("helper-controls");

const terminal = new Terminal({
  allowProposedApi: false,
  convertEol: false,
  cursorBlink: true,
  cursorStyle: "bar",
  disableStdin: true,
  fontFamily: '"SFMono-Regular", "SF Mono", Menlo, Consolas, monospace',
  fontSize: 14,
  lineHeight: 1.16,
  scrollback: 5_000,
  theme: {
    background: "#07100b",
    foreground: "#d9e7de",
    cursor: "#7ee2a8",
    cursorAccent: "#07100b",
    selectionBackground: "#295b3d",
    black: "#101713",
    brightBlack: "#637068",
    green: "#65d591",
    brightGreen: "#91efb5",
  },
});
const fitAddon = new FitAddon();
terminal.loadAddon(fitAddon);
terminal.open(terminalElement);

const credentials = loadCredentials(window.location, window.sessionStorage, (url) => window.history.replaceState(null, "", url));
if (!credentials) {
  setStatus("error", "Missing access token");
  terminal.writeln("\x1b[31mIvy could not find a valid Session token.\x1b[0m");
  terminal.writeln("Open the complete URL printed by Ivy and try again.");
  fitTerminal();
} else {
  const sizeTracker = new SizeTracker();
  let client: SessionClient;
  const helperControls = new HelperControls(
    helperControlsElement,
    (data) => client.sendInput(data),
    () => terminal.focus(),
  );
  client = new SessionClient({
    sessionID: credentials.sessionID,
    token: credentials.token,
    callbacks: {
      onState(state, label) {
        terminal.options.disableStdin = state !== "live";
        helperControls.setEnabled(state === "live");
        setStatus(state, label);
        if (state === "live") {
          scheduleFit(true);
          terminal.focus();
        }
      },
      onMetadata(metadata) {
        renderMetadata(metadata);
      },
      onReset() {
        terminal.reset();
      },
      onOutput(data) {
        terminal.write(data);
      },
      onExit() {
        clearCredential(window.sessionStorage, credentials.sessionID);
        terminal.options.disableStdin = true;
      },
      onPermanentFailure(message, shouldClearCredential) {
        if (shouldClearCredential) {
          clearCredential(window.sessionStorage, credentials.sessionID);
        }
        terminal.options.disableStdin = true;
        terminal.writeln(`\r\n\x1b[31mIvy: ${message}\x1b[0m`);
      },
    },
  });

  const encoder = new TextEncoder();
  terminal.onData((data) => client.sendInput(encoder.encode(data)));
  terminal.onBinary((data) => {
    const bytes = Uint8Array.from(data, (character) => character.charCodeAt(0) & 0xff);
    client.sendInput(bytes);
  });
  terminalElement.addEventListener("pointerdown", () => terminal.focus());

  const resizeObserver = new ResizeObserver(() => scheduleFit(false));
  resizeObserver.observe(terminalElement);
  window.addEventListener("resize", () => scheduleFit(false));
  window.addEventListener("orientationchange", () => scheduleFit(false));
  window.visualViewport?.addEventListener("resize", updateViewport);
  window.visualViewport?.addEventListener("scroll", updateViewport);

  let fitFrame: number | null = null;
  function scheduleFit(force: boolean): void {
    if (fitFrame !== null) {
      cancelAnimationFrame(fitFrame);
    }
    fitFrame = requestAnimationFrame(() => {
      fitFrame = null;
      fitTerminal();
      if (sizeTracker.shouldSend(terminal.cols, terminal.rows, force)) {
        client.sendResize(terminal.cols, terminal.rows);
      }
    });
  }

  window.addEventListener("pagehide", () => {
    resizeObserver.disconnect();
    helperControls.dispose();
    client.dispose();
  });

  updateViewport();
  scheduleFit(false);
  client.start();
}

function requiredElement(id: string): HTMLElement {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`missing #${id}`);
  }
  return element;
}

function renderMetadata(metadata: SessionMetadata): void {
  commandElement.textContent = metadata.command || "Terminal";
  directoryElement.textContent = metadata.directory;
  document.title = `${metadata.command || "Terminal"} · Ivy`;
}

function setStatus(state: ClientState, label: string): void {
  statusElement.className = `status status--${state}`;
  statusLabelElement.textContent = label;
}

function updateViewport(): void {
  const viewport = window.visualViewport;
  const height = viewport?.height ?? window.innerHeight;
  const top = viewport?.offsetTop ?? 0;
  document.documentElement.style.setProperty("--ivy-viewport-height", `${height}px`);
  document.documentElement.style.setProperty("--ivy-viewport-top", `${top}px`);
}

function fitTerminal(): void {
  try {
    fitAddon.fit();
  } catch {
    // A zero-sized viewport during rotation will trigger another resize.
  }
}
