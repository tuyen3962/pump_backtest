import { useEffect, useMemo, useRef, useState } from "react";
import { fetchFollow, type FollowRow } from "../api";

const FALLBACK_POLL_MS = 15_000;

export type FollowStreamState = {
  items: FollowRow[];
  byMint: Record<string, FollowRow>;
  updatedAt: string;
  connected: boolean;
  error: string;
  refresh: () => void;
};

export function useFollowStream(resetKey = 0): FollowStreamState {
  const [items, setItems] = useState<FollowRow[]>([]);
  const [updatedAt, setUpdatedAt] = useState("");
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState("");
  const [tick, setTick] = useState(0);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    let cancelled = false;
    let pollTimer: number | undefined;
    let usePoll = false;

    const apply = (data: { items?: FollowRow[]; updatedAt?: string; error?: string }) => {
      if (cancelled) return;
      setItems(data.items || []);
      setUpdatedAt(data.updatedAt || new Date().toISOString());
      if (data.error) setError(data.error);
      else setError("");
    };

    const startPoll = () => {
      if (usePoll) return;
      usePoll = true;
      setConnected(false);
      const poll = () => {
        void fetchFollow()
          .then(apply)
          .catch((err) => {
            if (!cancelled) setError(String((err as Error).message || err));
          });
      };
      poll();
      pollTimer = window.setInterval(poll, FALLBACK_POLL_MS);
    };

    if (typeof EventSource === "undefined") {
      startPoll();
      return () => {
        cancelled = true;
        if (pollTimer) window.clearInterval(pollTimer);
      };
    }

    const es = new EventSource("/api/follow/stream");
    esRef.current = es;
    es.onopen = () => {
      if (!cancelled) {
        setConnected(true);
        setError("");
      }
    };
    es.onmessage = (ev) => {
      try {
        apply(JSON.parse(ev.data));
      } catch (err) {
        if (!cancelled) setError(String((err as Error).message || err));
      }
    };
    es.onerror = () => {
      es.close();
      esRef.current = null;
      if (!cancelled) startPoll();
    };

    return () => {
      cancelled = true;
      es.close();
      esRef.current = null;
      if (pollTimer) window.clearInterval(pollTimer);
    };
  }, [resetKey, tick]);

  const byMint = useMemo(() => {
    const m: Record<string, FollowRow> = {};
    for (const row of items) {
      if (row.mint) m[row.mint] = row;
    }
    return m;
  }, [items]);

  return {
    items,
    byMint,
    updatedAt,
    connected,
    error,
    refresh: () => setTick((n) => n + 1),
  };
}
