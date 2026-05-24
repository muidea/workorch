import React from 'react';
import { Card, Layout, Menu, Spin, Table, Typography } from 'antd';
import type { CardProps, TableProps } from 'antd';

type RouteItem = {
  path?: string;
  name?: string;
  icon?: React.ReactNode;
};

type ProLayoutProps = {
  title?: React.ReactNode;
  logo?: React.ReactNode;
  route?: { path?: string; routes?: RouteItem[] };
  location?: { pathname?: string };
  children?: React.ReactNode;
  menuItemRender?: (item: RouteItem, dom: React.ReactNode) => React.ReactNode;
  layout?: string;
  contentWidth?: string;
};

export function ProLayout({ title, logo: _logo, route, location, children, menuItemRender, layout: _layout, contentWidth: _contentWidth }: ProLayoutProps) {
  const selectedPath = location?.pathname || route?.routes?.[0]?.path || '/';
  const items = (route?.routes ?? []).map((item) => {
    const label = menuItemRender ? menuItemRender(item, item.name) : item.name;
    return {
      key: item.path || item.name || '',
      icon: item.icon,
      label
    };
  });

  return (
    <Layout className="workorch-console-layout">
      <Layout.Sider className="workorch-console-sider" width={224} breakpoint="lg" collapsedWidth={0}>
        <div className="workorch-console-title">{title}</div>
        <Menu mode="inline" selectedKeys={[selectedPath]} items={items} />
      </Layout.Sider>
      <Layout className="workorch-console-main">
        <Layout.Content className="workorch-console-content">{children}</Layout.Content>
      </Layout>
    </Layout>
  );
}

type PageContainerProps = {
  title?: React.ReactNode;
  subTitle?: React.ReactNode;
  extra?: React.ReactNode;
  children?: React.ReactNode;
};

export function PageContainer({ title, subTitle, extra, children }: PageContainerProps) {
  return (
    <section className="workorch-page-container">
      {(title || subTitle || extra) && (
        <header className="workorch-page-header">
          <div className="workorch-page-title-block">
            {title && <Typography.Title level={3}>{title}</Typography.Title>}
            {subTitle && <Typography.Text type="secondary">{subTitle}</Typography.Text>}
          </div>
          {extra && <div className="workorch-page-extra">{extra}</div>}
        </header>
      )}
      <div className="workorch-page-body">{children}</div>
    </section>
  );
}

type ProCardColSpan = string | number | Record<string, string | number>;

type ProCardProps = Omit<CardProps, 'children'> & {
  children?: React.ReactNode;
  colSpan?: ProCardColSpan;
  gutter?: number;
  wrap?: boolean;
  ghost?: boolean;
  split?: 'vertical' | 'horizontal';
  subTitle?: React.ReactNode;
};

function flexBasisFromColSpan(colSpan?: ProCardColSpan) {
  if (colSpan == null) {
    return undefined;
  }
  if (typeof colSpan === 'string') {
    return colSpan.endsWith('%') ? colSpan : undefined;
  }
  if (typeof colSpan === 'number') {
    if (colSpan <= 0) {
      return undefined;
    }
    if (colSpan <= 24) {
      return `${(colSpan / 24) * 100}%`;
    }
    return `${colSpan}px`;
  }
  const large = colSpan.lg ?? colSpan.md ?? colSpan.sm ?? colSpan.xs;
  return flexBasisFromColSpan(large);
}

export function ProCard({ children, colSpan, gutter, wrap, ghost, split, className, style, loading, title, subTitle, ...props }: ProCardProps) {
  const itemStyle: React.CSSProperties = {
    ...style,
    flex: colSpan ? `1 1 ${flexBasisFromColSpan(colSpan) ?? 'auto'}` : style?.flex,
    minWidth: colSpan ? 0 : style?.minWidth
  };
  const body = gutter || split ? (
    <div
      className={[
        'workorch-pro-card-grid',
        wrap ? 'workorch-pro-card-grid-wrap' : '',
        split ? `workorch-pro-card-split-${split}` : ''
      ].filter(Boolean).join(' ')}
      style={{ gap: gutter ?? undefined }}
    >
      {children}
    </div>
  ) : children;

  if (ghost) {
    return (
      <div className={['workorch-pro-card-ghost', className].filter(Boolean).join(' ')} style={itemStyle}>
        {loading ? <Spin>{body}</Spin> : body}
      </div>
    );
  }
  const cardTitle = subTitle ? (
    <div className="workorch-pro-card-title">
      <span>{title}</span>
      <Typography.Text type="secondary">{subTitle}</Typography.Text>
    </div>
  ) : title;
  return (
    <Card className={['workorch-pro-card', className].filter(Boolean).join(' ')} style={itemStyle} loading={loading} title={cardTitle} {...props}>
      {body}
    </Card>
  );
}

export type ProColumns<T> = NonNullable<TableProps<T>['columns']>[number] & {
  hideInTable?: boolean;
  copyable?: boolean;
  valueType?: string;
};

type ProTableProps<T extends object> = Omit<TableProps<T>, 'columns'> & {
  columns?: ProColumns<T>[];
  search?: boolean;
  options?: boolean;
};

export function ProTable<T extends object>({ columns, search: _search, options: _options, ...props }: ProTableProps<T>) {
  const visibleColumns = columns?.filter((column) => !column.hideInTable);
  return <Table<T> columns={visibleColumns as TableProps<T>['columns']} {...props} />;
}
