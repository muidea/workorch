import { App as AntApp, ConfigProvider } from 'antd';
import { StyleProvider } from '@ant-design/cssinjs';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { WorkspaceTemplateManager } from '../../src/helpers';
import { workorchApi } from '../../src/api';

vi.mock('../../src/api', () => ({ workorchApi: {
  updateWorkspaceTemplate: vi.fn(), createWorkspaceTemplate: vi.fn(), deleteWorkspaceTemplate: vi.fn(), listWorkspaceTemplates: vi.fn()
} }));

const styleContainer = document.createElement('div');

it('内置模板只读查看，不能保存或删除，自定义模板仍可编辑', async () => {
  render(<StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}><AntApp><WorkspaceTemplateManager souls={[]} templates={[
    { id: 'system-template', name: '内置模板', builtin: true, goalText: '当前版本内容' },
    { id: 'user-template', name: '自定义模板' }
  ]} onTemplatesChange={() => {}} /></AntApp></ConfigProvider></StyleProvider>);
  expect(screen.getByText('系统内置 · 随版本更新')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: '查看' }));
  await act(async () => {});
  expect((screen.getByRole('button', { name: '系统内置模板只读' }) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByDisplayValue('当前版本内容') as HTMLTextAreaElement).disabled).toBe(true);
  expect((screen.getAllByRole('button', { name: '删除' })[0] as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole('button', { name: '编辑' }));
  await act(async () => {});
  expect((screen.getByRole('button', { name: '保存模板' }) as HTMLButtonElement).disabled).toBe(false);
  expect((screen.getAllByRole('button', { name: '删除' })[1] as HTMLButtonElement).disabled).toBe(false);
  expect(workorchApi.updateWorkspaceTemplate).not.toHaveBeenCalled();
  expect(workorchApi.deleteWorkspaceTemplate).not.toHaveBeenCalled();
  expect(workorchApi.createWorkspaceTemplate).not.toHaveBeenCalled();
});
