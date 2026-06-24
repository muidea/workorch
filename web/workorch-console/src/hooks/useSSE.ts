import { useEffect, useRef } from 'react';
import { createWorkorchEventStream } from '../api';

export interface SSEEvent {
  id?: string;
  topic: string;
  workspaceId?: string;
  runId?: string;
  revision?: number;
  payload?: Record<string, unknown>;
  stateSummary?: Record<string, unknown>;
}

export interface UseSSEOptions {
  enabled?: boolean;
  debounceMs?: number;
  onReconnect?: () => void;
  onExhausted?: (detail: { retryCount: number }) => void;
  maxRetries?: number;
  workspaceId?: string;
  runId?: string;
}

export function useSSE(onEvent: (event: SSEEvent) => void, options: UseSSEOptions = {}) {
  const { enabled = true, debounceMs = 150, onReconnect, onExhausted, maxRetries = 20, workspaceId, runId } = options;
  const onEventRef = useRef(onEvent);
  const onReconnectRef = useRef(onReconnect);
  const onExhaustedRef = useRef(onExhausted);
  const debounceTimerRef = useRef<number | undefined>(undefined);
  const retryCountRef = useRef(0);
  const retryTimerRef = useRef<number | undefined>(undefined);
  const lastEventIdRef = useRef('');
  const exhaustedNotifiedRef = useRef(false);

  useEffect(() => { onEventRef.current = onEvent; }, [onEvent]);
  useEffect(() => { onReconnectRef.current = onReconnect; }, [onReconnect]);
  useEffect(() => { onExhaustedRef.current = onExhausted; }, [onExhausted]);
  useEffect(() => {
    lastEventIdRef.current = '';
    retryCountRef.current = 0;
    exhaustedNotifiedRef.current = false;
  }, [workspaceId, runId]);

  useEffect(() => {
    if (!enabled) return undefined;

    let stream: EventSource | null = null;
    let disposed = false;

    function connect() {
      if (disposed) return;
      stream = createWorkorchEventStream({ workspaceId, runId, lastEventId: lastEventIdRef.current });

      stream.addEventListener('workorch', (event: MessageEvent<string>) => {
        retryCountRef.current = 0;
        exhaustedNotifiedRef.current = false;
        try {
          const data = JSON.parse(event.data) as SSEEvent;
          data.id = event.lastEventId || data.id;
          if (data.id) {
            lastEventIdRef.current = data.id;
          }
          if (!data.revision && typeof data.payload?.stateSummary === 'object' && data.payload.stateSummary !== null) {
            const summary = data.payload.stateSummary as Record<string, unknown>;
            if (typeof summary.revision === 'number') {
              data.revision = summary.revision;
            }
            data.stateSummary = summary;
          }
          if (debounceTimerRef.current !== undefined) {
            window.clearTimeout(debounceTimerRef.current);
          }
          debounceTimerRef.current = window.setTimeout(() => {
            debounceTimerRef.current = undefined;
            onEventRef.current(data);
          }, debounceMs);
        } catch {
          // ignore parse errors
        }
      });

      stream.onopen = () => {
        exhaustedNotifiedRef.current = false;
        if (retryCountRef.current > 0) {
          retryCountRef.current = 0;
          onReconnectRef.current?.();
        }
      };

      stream.onerror = () => {
        stream?.close();
        stream = null;
        if (disposed) return;
        onReconnectRef.current?.();
        const exhausted = retryCountRef.current >= maxRetries;
        if (exhausted && !exhaustedNotifiedRef.current) {
          exhaustedNotifiedRef.current = true;
          onExhaustedRef.current?.({ retryCount: retryCountRef.current });
        }
        const delay = nextSSERetryDelayMs(retryCountRef.current);
        retryCountRef.current += 1;
        if (retryTimerRef.current !== undefined) {
          window.clearTimeout(retryTimerRef.current);
        }
        retryTimerRef.current = window.setTimeout(connect, delay);
      };
    }

    connect();

    return () => {
      disposed = true;
      stream?.close();
      if (debounceTimerRef.current !== undefined) {
        window.clearTimeout(debounceTimerRef.current);
      }
      if (retryTimerRef.current !== undefined) {
        window.clearTimeout(retryTimerRef.current);
      }
    };
  }, [enabled, debounceMs, maxRetries, workspaceId, runId]);
}

export function nextSSERetryDelayMs(retryCount: number) {
  const normalized = Number.isFinite(retryCount) && retryCount > 0 ? retryCount : 0;
  return Math.min(1000 * Math.pow(2, normalized), 30000);
}
