import { create } from 'zustand';
import type { WorkspaceSnapshot, Goal, DoD } from '../types';

export type WorkspaceBoardItem = {
  snapshot: WorkspaceSnapshot;
  goals: Goal[];
  dods: DoD[];
};

interface BoardState {
  items: WorkspaceBoardItem[];
  loading: boolean;

  setItems: (items: WorkspaceBoardItem[]) => void;
  mergeItems: (newItems: WorkspaceBoardItem[]) => void;
  setLoading: (loading: boolean) => void;
  reset: () => void;
}

export const useBoardStore = create<BoardState>((set) => ({
  items: [],
  loading: false,

  setItems: (items) => set({ items }),

  mergeItems: (newItems) =>
    set((state) => {
      const merged = [...state.items];
      for (const newItem of newItems) {
        const index = merged.findIndex((item) => item.snapshot.workspace.id === newItem.snapshot.workspace.id);
        if (index >= 0) {
          merged[index] = newItem;
        } else {
          merged.push(newItem);
        }
      }
      return { items: merged };
    }),

  setLoading: (loading) => set({ loading }),

  reset: () => set({ items: [], loading: false }),
}));
