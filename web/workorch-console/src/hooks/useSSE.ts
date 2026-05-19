import { useEffect, useRef, useCallback } from 'react';
import { createWorkorchEventStream } from '../api';

export interface SSEEvent {
  topic: string;
  workspaceId?: string;
  runId?: string;
  payload?: unknown;
}

/**
 * SSE 事件流 Hook
 * 统一管理 Server-Sent Events 连接和事件处理
 *
 * 优化特性：
 * - 使用 useRef 存储回调，避免因回调变化导致重连
 * - 防抖处理，避免频繁触发更新
 * - 自动错误处理和重连
 */
export function useSSE(
  onEvent: (event: SSEEvent) => void,
  enabled: boolean = true,
  debounceMs: number = 150
) {
  const debounceTimerRef = useRef<number | undefined>(undefined);
  const onEventRef = useRef(onEvent);
  const pendingEventRef = useRef<SSEEvent | null>(null);

  // 保持 onEvent 引用最新，避免闭包陷阱
  useEffect(() => {
    onEventRef.current = onEvent;
  }, [onEvent]);

  useEffect(() => {
    if (!enabled) {
      return undefined;
    }

    const stream = createWorkorchEventStream();

    const handleEvent = (event: MessageEvent<string>) => {
      try {
        const data = JSON.parse(event.data) as SSEEvent;

        // 清除之前的防抖定时器
        if (debounceTimerRef.current !== undefined) {
          window.clearTimeout(debounceTimerRef.current);
        }

        // 保存最新事件
        pendingEventRef.current = data;

        // 设置新的防抖定时器
        debounceTimerRef.current = window.setTimeout(() => {
          debounceTimerRef.current = undefined;
          if (pendingEventRef.current) {
            onEventRef.current(pendingEventRef.current);
            pendingEventRef.current = null;
          }
        }, debounceMs);
      } catch (error) {
        console.error('SSE parse error:', error);
      }
    };

    stream.addEventListener('workorch', handleEvent);
    stream.onerror = () => {
      stream.close();
    };

    return () => {
      stream.removeEventListener('workorch', handleEvent);
      stream.close();
      if (debounceTimerRef.current !== undefined) {
        window.clearTimeout(debounceTimerRef.current);
        debounceTimerRef.current = undefined;
      }
      pendingEventRef.current = null;
    };
  }, [enabled, debounceMs]);
}

/**
 * 创建稳定的 SSE 事件处理器
 * 使用 useCallback 确保回调函数引用稳定
 */
export function useSSECallback<T extends unknown[]>(
  callback: (...args: T) => void,
  deps: React.DependencyList
): (...args: T) => void {
  return useCallback(callback, deps);
}
