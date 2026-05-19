import { create } from 'zustand';

export type ViewKey = 'board' | 'workspaceDetail' | 'governance' | 'runtimeControl' | 'configuration' | 'service';

interface ViewState {
  currentView: ViewKey;
  selectedWorkspaceId: string | undefined;

  setView: (view: ViewKey) => void;
  selectWorkspace: (id: string | undefined) => void;
}

export const useViewStore = create<ViewState>((set) => ({
  currentView: 'board',
  selectedWorkspaceId: undefined,

  setView: (view) => set({ currentView: view }),
  selectWorkspace: (id) => set({ selectedWorkspaceId: id }),
}));
