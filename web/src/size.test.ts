import { SizeTracker } from "./size";

describe("SizeTracker", () => {
  it("deduplicates dimensions unless a live connection requests the current size", () => {
    const tracker = new SizeTracker();
    expect(tracker.shouldSend(100, 30)).toBe(true);
    expect(tracker.shouldSend(100, 30)).toBe(false);
    expect(tracker.shouldSend(100, 30, true)).toBe(true);
    expect(tracker.shouldSend(101, 30)).toBe(true);
  });
});
