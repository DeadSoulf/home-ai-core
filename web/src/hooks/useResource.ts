import { useCallback, useEffect, useState } from "react";

export type ResourceState<T> = {
  data?: T;
  loading: boolean;
  error?: string;
  reload: () => void;
};

export function useResource<T>(
  load: () => Promise<T>,
  revision = 0,
): ResourceState<T> {
  const [data, setData] = useState<T>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [nonce, setNonce] = useState(0);

  const reload = useCallback(() => setNonce((value) => value + 1), []);

  useEffect(() => {
    let active = true;
    setLoading(true);

    load()
      .then((value) => {
        if (!active) return;
        setData(value);
        setError(undefined);
      })
      .catch((reason: unknown) => {
        if (!active) return;
        setError(reason instanceof Error ? reason.message : "Request failed");
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [load, nonce, revision]);

  return {data, loading, error, reload};
}
