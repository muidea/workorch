import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type { Decision, DecisionPage } from '../types';
import { workorchApi } from '../api';
import { mergeById } from '../utils';

const governancePageLimit = 20;

interface DecisionState {
  selectedKeys: React.Key[];
  target: { item: Decision; resolutionType: string } | undefined;
  submitting: boolean;
  records: Decision[];
  cursorAfterId: string | undefined;
  hasMore: boolean;
  loading: boolean;

  setSelectedKeys: (keys: React.Key[]) => void;
  setTarget: (target: { item: Decision; resolutionType: string } | undefined) => void;
  setSubmitting: (submitting: boolean) => void;
  applyPage: (page: DecisionPage, append: boolean) => void;
  loadPage: (cursorAfterId?: string) => Promise<void>;
  reset: () => void;
}

export const useDecisionStore = create<DecisionState>()(
  devtools(
    (set, get) => ({
      selectedKeys: [],
      target: undefined,
      submitting: false,
      records: [],
      cursorAfterId: undefined,
      hasMore: false,
      loading: false,

      setSelectedKeys: (keys) => set({ selectedKeys: keys }),
      setTarget: (target) => set({ target }),
      setSubmitting: (submitting) => set({ submitting }),

      applyPage: (page, append) => {
        const items = (page.decisions ?? []).sort(
          (left, right) => Number(left.state !== 'open') - Number(right.state !== 'open')
        );
        set((state) => ({
          records: mergeById(state.records, items, append),
          cursorAfterId: page.nextCursorAfterId || items.at(-1)?.id,
          hasMore: Boolean(page.hasMore),
        }));
      },

      loadPage: async (cursorAfterId) => {
        set({ loading: true });
        try {
          const page = await workorchApi.listDecisionPage({ cursorAfterId, limit: governancePageLimit });
          get().applyPage(page, Boolean(cursorAfterId));
        } finally {
          set({ loading: false });
        }
      },

      reset: () =>
        set({
          selectedKeys: [],
          target: undefined,
          submitting: false,
          records: [],
          cursorAfterId: undefined,
          hasMore: false,
          loading: false,
        }),
    }),
    { name: 'DecisionStore' }
  )
);
