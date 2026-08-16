# WorkOrch Web Console Bundle And UI Optimization Record

Status: archived

Type: historical-record

Archived: 2026-08-16

> Historical measurement and design record. It does not define current bundle thresholds or UI architecture.

## Recorded Problem

At the time of this optimization, the console had two issues:

- `vendor-antd-*` and the main entry carried unnecessary code;
- `ProCard` was used both as visible content and as a layout container, producing excessive visual nesting.

## Decisions Taken

- removed the aggregate `@ant-design/pro-components` dependency;
- introduced a narrow project-local compatibility layer for the page features then in use;
- lazy-loaded page components and split chunks at actual cache boundaries;
- kept `PageContainer` as page shell, `CardGrid` as layout, and visible `ProCard` for titled/bordered groups;
- removed layout-only `ProCard gutter/wrap/split` usages.

## Measurement Recorded At Completion

- transformed modules: approximately 9,879 to 3,120;
- main entry: approximately 280 KB / gzip 85 KB to 7 KB / gzip 3 KB;
- coarse `vendor-antd` became a smaller `vendor-antd-runtime` boundary;
- `@ant-design/pro-components`, `.ant-pro-*`, and layout-only ProCard usages were absent in that build.

These values are a point-in-time result, not a current performance baseline.

## Historical Verification Commands

```bash
npm --prefix web/workorch-console run build
git diff --check
rg -n "@ant-design/pro-components|ant-pro" web/workorch-console web/consoleassets/dist
rg -n "<ProCard[^>]*gutter=|<ProCard[^>]*split=|<ProCard[^>]*wrap" web/workorch-console/src
```

Reusable guidance: prefer basic components, keep shell/layout/content responsibilities distinct, and measure actual generated
chunks before setting a new current threshold.
