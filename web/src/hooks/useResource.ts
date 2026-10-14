import { useCallback, useEffect, useRef, useState } from "react";
import { useI18n } from "../i18n";
import { errorText } from "../ui/messages";
import { resourcePoller } from "./resourcePoller";

export type ResourceState<T> = {
  data?: T; loading: boolean; slow: boolean; error?: string; failure?: unknown;
  updatedAt?: number; reload: () => void;
};

export function useResource<T>(load: (signal: AbortSignal) => Promise<T>, revision = 0, pollInterval = 0): ResourceState<T> {
  const {locale} = useI18n();
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [slow, setSlow] = useState(false);
  const [failure, setFailure] = useState<unknown>();
  const [updatedAt, setUpdatedAt] = useState<number>();
  const refresh = useRef<() => void>(() => {});
  const previousRevision = useRef(revision);
  const reload = useCallback(() => refresh.current(), []);

  useEffect(() => {
    let slowTimer: ReturnType<typeof setTimeout> | undefined;
    const poller = resourcePoller(load, {
      start() { setLoading(true); setSlow(false); slowTimer = setTimeout(() => setSlow(true), 8000); },
      success(value) { setData(value); setFailure(undefined); setUpdatedAt(Date.now()); },
      failure(reason) { setFailure(reason); },
      finish() { clearTimeout(slowTimer); setLoading(false); setSlow(false); },
    }, pollInterval);
    refresh.current = poller.refresh;
    poller.refresh();
    return () => { poller.dispose(); clearTimeout(slowTimer); refresh.current = () => {}; };
  }, [load, pollInterval]);

  useEffect(() => {
    if (previousRevision.current !== revision) { previousRevision.current = revision; reload(); }
  }, [revision, reload]);
  return {data, loading, slow, failure, error: failure === undefined ? undefined : errorText(failure, locale), updatedAt, reload};
}
