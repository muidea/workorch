import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AuthoringSummary } from '../../src/pages/components/AuthoringSummary';

describe('AuthoringSummary', () => {
  it('显示创作失败阶段、审查数量并提供规格入口，不混淆 Run 状态', () => {
    const onOpenSpecification = vi.fn();
    render(<AuthoringSummary
      summary={{ state: 'preparation_failed', failurePhase: 'derivation', failureCode: 'provider_unavailable', reviewCount: 2, inputRevision: 4 }}
      onOpenSpecification={onOpenSpecification}
    />);
    expect(screen.getByText('创作准备失败')).toBeTruthy();
    expect(screen.getByText('失败阶段：派生')).toBeTruthy();
    expect(screen.getByText('已完成审查：2 项')).toBeTruthy();
    expect(screen.getByText(/不代表执行（Run）状态/)).toBeTruthy();
    fireEvent.click(screen.getByText('进入规格查看'));
    expect(onOpenSpecification).toHaveBeenCalledOnce();
  });

  it('展示已持久的方案失败，不推断 Run 或确认能力', () => {
    render(<AuthoringSummary
      summary={{ state: 'failed', failurePhase: 'proposal', failureCode: 'invalid_proposal', reviewCount: 0, inputRevision: 1 }}
    />);
    expect(screen.getByText('创作失败')).toBeTruthy();
    expect(screen.getByText('失败阶段：方案生成')).toBeTruthy();
    expect(screen.getByText('原因代码：方案无效')).toBeTruthy();
    expect(screen.getByText('已完成审查：0 项')).toBeTruthy();
    expect(screen.getByText(/不代表执行（Run）状态/)).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('摘要缺失时不渲染内容', () => {
    const { container } = render(<AuthoringSummary />);
    expect(container.firstChild).toBeNull();
  });
});
