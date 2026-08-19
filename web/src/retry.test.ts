import { retryDelay } from "./retry";

describe("retryDelay", () => {
  it("uses the bounded reconnect schedule", () => {
    expect([0, 1, 2, 3, 4, 5, 100].map(retryDelay)).toEqual([250, 500, 1_000, 2_000, 5_000, 5_000, 5_000]);
  });
});
