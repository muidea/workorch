import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type { Workspace, WorkspaceSnapshot, Goal, DoD, Skill, Soul, WorkspaceSoulBindings } from '../types';

export type WorkspaceSetupMode = 'manual' | 'smart';

interface WorkspaceState {
  // 数据
  workspaces: Workspace[];
  selectedId: string | undefined;
  snapshot: WorkspaceSnapshot | undefined;
  goals: Goal[];
  dods: DoD[];
  souls: Soul[];
  soulBindings: WorkspaceSoulBindings | undefined;
  skills: Skill[];

  // UI 状态
  deleteOpen: boolean;
  listReady: boolean;
  setupStep: number;
  setupMode: WorkspaceSetupMode;

  // 加载状态
  loading: boolean;
  error: string | undefined;

  // 操作
  setWorkspaces: (workspaces: Workspace[]) => void;
  mergeWorkspaces: (newWorkspaces: Workspace[]) => void;
  selectWorkspace: (id: string | undefined) => void;
  setSnapshot: (snapshot: WorkspaceSnapshot | undefined) => void;
  setGoals: (goals: Goal[]) => void;
  setDoDs: (dods: DoD[]) => void;
  setSouls: (souls: Soul[]) => void;
  setSoulBindings: (bindings: WorkspaceSoulBindings | undefined) => void;
  setSkills: (skills: Skill[]) => void;
  setDeleteOpen: (open: boolean) => void;
  setListReady: (ready: boolean) => void;
  setSetupStep: (step: number | ((current: number) => number)) => void;
  setSetupMode: (mode: WorkspaceSetupMode) => void;
  clearWorkspaceContext: () => void;
  reset: () => void;
}

export const useWorkspaceStore = create<WorkspaceState>()(
  devtools(
    (set) => ({
      // 初始状态
      workspaces: [],
      selectedId: undefined,
      snapshot: undefined,
      goals: [],
      dods: [],
      souls: [],
      soulBindings: undefined,
      skills: [],
      deleteOpen: false,
      listReady: false,
      setupStep: 0,
      setupMode: 'manual',
      loading: false,
      error: undefined,

      // 设置工作空间列表
      setWorkspaces: (workspaces) => set({ workspaces }),

      // 合并工作空间（用于 SSE 增量更新）
      mergeWorkspaces: (newWorkspaces) =>
        set((state) => {
          const merged = [...state.workspaces];
          for (const newWs of newWorkspaces) {
            const index = merged.findIndex((ws) => ws.id === newWs.id);
            if (index >= 0) {
              merged[index] = newWs;
            } else {
              merged.push(newWs);
            }
          }
          return { workspaces: merged };
        }),

      // 选择工作空间
      selectWorkspace: (id) => set({ selectedId: id }),

      // 设置快照
      setSnapshot: (snapshot) => set({ snapshot }),

      // 设置目标
      setGoals: (goals) => set({ goals }),

      // 设置 DoD
      setDoDs: (dods) => set({ dods }),

      // 设置 Soul
      setSouls: (souls) => set({ souls }),

      // 设置 Soul 绑定
      setSoulBindings: (bindings) => set({ soulBindings: bindings }),

      // 设置技能
      setSkills: (skills) => set({ skills }),

      // 设置删除对话框状态
      setDeleteOpen: (open) => set({ deleteOpen: open }),

      // 设置列表就绪状态
      setListReady: (ready) => set({ listReady: ready }),

      // 设置设置步骤
      setSetupStep: (step) => set((state) => ({
        setupStep: typeof step === 'function' ? step(state.setupStep) : step
      })),

      // 设置设置模式
      setSetupMode: (mode) => set({ setupMode: mode }),

      // 清除工作空间上下文
      clearWorkspaceContext: () =>
        set({
          snapshot: undefined,
          goals: [],
          dods: [],
          soulBindings: undefined,
          skills: [],
        }),

      // 重置所有状态
      reset: () =>
        set({
          workspaces: [],
          selectedId: undefined,
          snapshot: undefined,
          goals: [],
          dods: [],
          souls: [],
          soulBindings: undefined,
          skills: [],
          deleteOpen: false,
          listReady: false,
          setupStep: 0,
          setupMode: 'manual',
          loading: false,
          error: undefined,
        }),
    }),
    { name: 'WorkspaceStore' }
  )
);
