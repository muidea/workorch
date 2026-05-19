import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type { Approval, ApprovalPage, Decision } from '../types';
import { workorchApi } from '../api';
import { mergeById } from '../utils';

const governancePageLimit = 20;

interface ApprovalState {
  selectedKeys: React.Key[];
  target: { item: Approval; action: 'approve' | 'reject'; relatedDecision?: Decision } | undefined;
  submitting: boolean;
  records: Approval[];
  cursorAfterId: string | undefined;
  hasMore: boolean;
  loading: boolean;

  setSelectedKeys: (keys: React.Key[]) => void;
  setTarget: (target: { item: Approval; action: 'approve' | 'reject'; relatedDecision?: Decision } | undefined) => void;
  setSubmitting: (submitting: boolean) => void;
  applyPage: (page: ApprovalPage, append: boolean) => void;
  loadPage: (cursorAfterId?: string) => Promise<void>;
  reset: () => void;
}

export const useApprovalStore = create<ApprovalState>()(
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
        const items = (page.approvals ?? []).sort(
          (left, right) => Number(left.state !== 'pending') - Number(right.state !== 'pending')
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
          const page = await workorchApi.listApprovalPage({ cursorAfterId, limit: governancePageLimit });
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
    { name: 'ApprovalStore' }
  )
);
