import { useEffect, useRef } from 'react';
import { createWorkorchEventStream } from '../api';

export interface SSEEvent {
  topic: string;
  workspaceId?: string;
  runId?: string;
  payload?: unknown;
}

export interface UseSSEOptions {
  enabled?: boolean;
  debounceMs?: number;
  onReconnect?: () => void;
  maxRetries?: number;
}

export function useSSE(onEvent: (event: SSEEvent) => void, options: UseSSEOptions = {}) {
  const { enabled = true, debounceMs = 150, onReconnect, maxRetries = 20 } = options;
  const onEventRef = useRef(onEvent);
  const onReconnectRef = useRef(onReconnect);
  const debounceTimerRef = useRef<number | undefined>(undefined);
  const retryCountRef = useRef(0);
  const retryTimerRef = useRef<number | undefined>(undefined);

  useEffect(() => { onEventRef.current = onEvent; }, [onEvent]);
  useEffect(() => { onReconnectRef.current = onReconnect; }, [onReconnect]);

  useEffect(() => {
    if (!enabled) return undefined;

    let stream: EventSource | null = null;
    let disposed = false;

    function connect() {
      if (disposed) return;
      stream = createWorkorchEventStream();

      stream.addEventListener('workorch', (event: MessageEvent<string>) => {
        retryCountRef.current = 0;
        try {
          const data = JSON.parse(event.data) as SSEEvent;
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
        if (retryCountRef.current > 0) {
          retryCountRef.current = 0;
          onReconnectRef.current?.();
        }
      };

      stream.onerror = () => {
        stream?.close();
        stream = null;
        if (disposed) return;
        if (retryCountRef.current >= maxRetries) return;
        const delay = Math.min(1000 * Math.pow(2, retryCountRef.current), 30000);
        retryCountRef.current += 1;
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
  }, [enabled, debounceMs, maxRetries]);
}
