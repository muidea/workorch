import { useState, useCallback } from 'react';
import { mergeById } from '../utils/format';

export interface PaginationResult<T> {
  items: T[];
  nextCursor?: string;
  hasMore: boolean;
}

export interface UsePaginationReturn<T> {
  records: T[];
  loading: boolean;
  hasMore: boolean;
  refresh: () => Promise<void>;
  loadMore: () => Promise<void>;
  reset: () => void;
}

/**
 * 通用分页 Hook
 * 用于处理游标分页逻辑，消除重复的分页加载代码
 */
export function usePagination<T extends { id: string }>(
  fetcher: (cursor?: string, limit?: number) => Promise<PaginationResult<T>>,
  limit: number = 100
): UsePaginationReturn<T> {
  const [records, setRecords] = useState<T[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);

  const load = useCallback(
    async (append: boolean = false) => {
      setLoading(true);
      try {
        const result = await fetcher(append ? cursor : undefined, limit);
        setRecords((prev) => (append ? mergeById(prev, result.items, true) : result.items));
        setCursor(result.nextCursor);
        setHasMore(result.hasMore);
      } finally {
        setLoading(false);
      }
    },
    [fetcher, cursor, limit]
  );

  const refresh = useCallback(() => load(false), [load]);
  const loadMore = useCallback(() => load(true), [load]);

  const reset = useCallback(() => {
    setRecords([]);
    setCursor(undefined);
    setHasMore(false);
  }, []);

  return { records, loading, hasMore, refresh, loadMore, reset };
}
