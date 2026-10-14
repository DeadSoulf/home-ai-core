// Coalesce refreshes without invalidating a useful in-flight response.
export function resourcePoller<T>(
  load: (signal: AbortSignal) => Promise<T>,
  callbacks: {start: () => void; success: (value: T) => void; failure: (error: unknown) => void; finish: () => void},
  interval = 0,
) {
  let stopped = false, inFlight = false, pending = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let controller: AbortController | undefined;
  async function refresh() {
    if (stopped) return;
    if (inFlight) { pending = true; return; }
    if (timer !== undefined) clearTimeout(timer);
    inFlight = true;
    controller = new AbortController();
    callbacks.start();
    try {
      const value = await load(controller.signal);
      if (!stopped) callbacks.success(value);
    } catch (error) {
      if (!stopped) callbacks.failure(error);
    } finally {
      inFlight = false;
      if (!stopped) {
        callbacks.finish();
        const delay = pending ? 0 : interval;
        if (pending || interval > 0) timer = setTimeout(() => { void refresh(); }, delay);
        pending = false;
      }
    }
  }
  return {
    refresh: () => { void refresh(); },
    dispose: () => { stopped = true; pending = false; if (timer !== undefined) clearTimeout(timer); controller?.abort(); },
  };
}
