export const protocolVersion = 1;
export const websocketProtocol = "ivy.v1";
export const authenticationPrefix = "ivy.auth.";

export interface SessionMetadata {
  id: string;
  command: string;
  directory: string;
  state: "running" | "exited";
  exitCode: number | null;
}

export interface HelloMessage {
  type: "hello";
  version: number;
  session: SessionMetadata;
}

export interface ExitMessage {
  type: "exit";
  code: number;
}

export interface ErrorMessage {
  type: "error";
  code: string;
  message: string;
}

export type ServerControl = HelloMessage | ExitMessage | ErrorMessage;

export function decodeMetadata(value: unknown, sessionID: string): SessionMetadata | null {
  if (!isRecord(value)) {
    return null;
  }
  const exitCode = value.exitCode;
  if (
    value.id !== sessionID ||
    typeof value.command !== "string" ||
    typeof value.directory !== "string" ||
    (value.state !== "running" && value.state !== "exited") ||
    (exitCode !== null && !Number.isInteger(exitCode))
  ) {
    return null;
  }
  return value as unknown as SessionMetadata;
}

export function decodeControl(data: string, sessionID: string): ServerControl | null {
  let value: unknown;
  try {
    value = JSON.parse(data);
  } catch {
    return null;
  }
  if (!isRecord(value) || typeof value.type !== "string") {
    return null;
  }
  switch (value.type) {
    case "hello": {
      const session = decodeMetadata(value.session, sessionID);
      if (value.version !== protocolVersion || !session) {
        return null;
      }
      return { type: "hello", version: value.version, session };
    }
    case "exit":
      return Number.isInteger(value.code) ? { type: "exit", code: value.code as number } : null;
    case "error":
      return typeof value.code === "string" && typeof value.message === "string"
        ? { type: "error", code: value.code, message: value.message }
        : null;
    default:
      return null;
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
