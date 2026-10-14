import { afterEach, describe, expect, it, vi } from "vitest";
import { resourcePoller } from "./resourcePoller";
afterEach(() => vi.useRealTimers());
describe("UI-01: sequential resource polling", () => {
  it("accepts a 2600ms response even when refreshes arrive every 2000ms", async () => {
    vi.useFakeTimers();
    let concurrent = 0, maximum = 0;
    const success = vi.fn();
    const load = vi.fn(() => new Promise<number>((resolve) => { maximum = Math.max(maximum, ++concurrent); setTimeout(() => { concurrent--; resolve(42); }, 2600); }));
    const poller = resourcePoller(load, {start: vi.fn(), success, failure: vi.fn(), finish: vi.fn()}, 2000);
    poller.refresh();
    await vi.advanceTimersByTimeAsync(2000); poller.refresh(); poller.refresh();
    await vi.advanceTimersByTimeAsync(700);
    expect(success).toHaveBeenCalledWith(42); expect(maximum).toBe(1);
    await vi.advanceTimersByTimeAsync(3000);
    expect(success.mock.calls.length).toBeGreaterThanOrEqual(2); expect(maximum).toBe(1);
    poller.dispose();
  });
  it("waits the polling interval after completion, not after request start", async () => {
    vi.useFakeTimers(); const load = vi.fn(async () => "ok");
    const poller = resourcePoller(load, {start: vi.fn(), success: vi.fn(), failure: vi.fn(), finish: vi.fn()}, 2000);
    poller.refresh(); await vi.advanceTimersByTimeAsync(1999); expect(load).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1); expect(load).toHaveBeenCalledTimes(2); poller.dispose();
  });
  it("does not update state or retry after unmount", async () => {
    vi.useFakeTimers(); let signal: AbortSignal | undefined;
    const success = vi.fn(), failure = vi.fn();
    const poller = resourcePoller((value) => { signal = value; return new Promise((resolve) => setTimeout(resolve, 1000)); }, {start: vi.fn(), success, failure, finish: vi.fn()}, 2000);
    poller.refresh(); poller.dispose(); await vi.advanceTimersByTimeAsync(5000);
    expect(signal?.aborted).toBe(true); expect(success).not.toHaveBeenCalled(); expect(failure).not.toHaveBeenCalled();
  });
  it("recovers after a failed poll", async () => {
    vi.useFakeTimers(); const success = vi.fn(), failure = vi.fn();
    const load = vi.fn().mockRejectedValueOnce(new Error("offline")).mockResolvedValue("recovered");
    const poller = resourcePoller(load, {start: vi.fn(), success, failure, finish: vi.fn()}, 2000);
    poller.refresh(); await vi.advanceTimersByTimeAsync(2001);
    expect(failure).toHaveBeenCalledTimes(1); expect(success).toHaveBeenCalledWith("recovered"); poller.dispose();
  });
});
