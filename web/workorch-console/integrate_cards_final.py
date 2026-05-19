#!/usr/bin/env python3
"""
卡片组件集成脚本 - 使用精确的字符串匹配和替换
"""

import sys
import re

def read_file(path):
    with open(path, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(path, content):
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

def add_imports(content):
    """步骤 1: 添加组件导入"""
    import_statement = """import {
  TaskProgressCard,
  TaskDecompositionCard,
  ExecutionSectionsCard,
  MemoryRecallCard,
  WebSourceCard,
  LLMUsageCard,
  LLMContextBudgetCard,
  ContextBundleCard
} from './components/cards';
"""

    # 在 ProCard 导入后插入
    old = "import { ProCard, ProTable } from '@ant-design/pro-components';"
    new = old + "\n" + import_statement

    return content.replace(old, new)

def replace_function_calls(content):
    """步骤 2: 替换函数调用为组件调用"""

    # 替换 1: TaskProgressCard (多行调用)
    content = content.replace(
        """{taskProgressCard(
              runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress,
              runObservability?.repairLoop ?? runSnapshot?.repairLoop
            )}""",
        """<TaskProgressCard
              progress={runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress}
              repairLoopOverride={runObservability?.repairLoop ?? runSnapshot?.repairLoop}
            />"""
    )

    # 替换 2: TaskDecompositionCard
    content = content.replace(
        "{taskDecompositionCard(runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions)}",
        "<TaskDecompositionCard items={runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions} />"
    )

    # 替换 3: ExecutionSectionsCard
    content = content.replace(
        "{executionSectionsCard(runObservability?.executionSections ?? runSnapshot?.executionSections ?? snapshot.executionSections)}",
        "<ExecutionSectionsCard items={runObservability?.executionSections ?? runSnapshot?.executionSections ?? snapshot.executionSections} />"
    )

    # 替换 4: MemoryRecallCard
    content = content.replace(
        "{memoryRecallCard(runSnapshot.memoryRecallTraces)}",
        "<MemoryRecallCard items={runSnapshot.memoryRecallTraces} />"
    )

    # 替换 5: WebSourceCard (两处)
    content = content.replace(
        "{webSourceCard(runWebSources)}",
        "<WebSourceCard items={runWebSources} />"
    )
    content = content.replace(
        "children: webSourceCard(runWebSources)",
        "children: <WebSourceCard items={runWebSources} />"
    )

    # 替换 6: LLMContextBudgetCard (两处)
    content = content.replace(
        "{llmContextBudgetCard(runLLMContextBudgets)}",
        "<LLMContextBudgetCard traces={runLLMContextBudgets} />"
    )
    content = content.replace(
        "children: llmContextBudgetCard(runLLMContextBudgets)",
        "children: <LLMContextBudgetCard traces={runLLMContextBudgets} />"
    )

    # 替换 7: LLMUsageCard (两处)
    content = content.replace(
        "{llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)}",
        "<LLMUsageCard summary={runLLMUsageSummary} traces={runLLMUsageTraces} />"
    )
    content = content.replace(
        "children: llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)",
        "children: <LLMUsageCard summary={runLLMUsageSummary} traces={runLLMUsageTraces} />"
    )

    # 替换 8: ContextBundleCard
    content = content.replace(
        "{contextBundleCard(contextBundle)}",
        "<ContextBundleCard bundle={contextBundle} />"
    )

    return content

def remove_function_definitions(content):
    """步骤 3: 删除旧函数定义"""

    # 定义每个函数的开始和结束标记
    functions = [
        ('function taskProgressCard(progress?: TaskProgress, repairLoopOverride?: RepairLoop) {',
         '\nfunction taskDecompositionCard(items?: TaskDecomposition[]) {'),

        ('function taskDecompositionCard(items?: TaskDecomposition[]) {',
         '\nfunction executionSectionsCard(items?: ExecutionSection[]) {'),

        ('function executionSectionsCard(items?: ExecutionSection[]) {',
         '\ntype MemoryRecallGroup = {'),

        ('type MemoryRecallGroup = {',
         '\nfunction memoryRecallCard(items?: MemoryRecallTrace[]) {'),

        ('function memoryRecallCard(items?: MemoryRecallTrace[]) {',
         '\ntype WebSourceRow = {'),

        ('type WebSourceRow = {',
         '\nfunction webSourceCard(items?: WebSourceTrace[]) {'),

        ('function webSourceCard(items?: WebSourceTrace[]) {',
         '\nfunction numberText(value?: number) {'),

        ('function llmUsageCard(summary?: LLMUsageSummary, traces?: LLMUsageTrace[]) {',
         '\nfunction llmContextBudgetCard(traces?: LLMContextBudgetTrace[]) {'),

        ('function llmContextBudgetCard(traces?: LLMContextBudgetTrace[]) {',
         '\nfunction contextBundleCard(bundle?: ContextBundle) {'),

        ('function contextBundleCard(bundle?: ContextBundle) {',
         '\nfunction decisionRequiredFields(item: Decision) {'),
    ]

    for start_marker, end_marker in functions:
        start_idx = content.find(start_marker)
        if start_idx != -1:
            end_idx = content.find(end_marker, start_idx)
            if end_idx != -1:
                # 删除从开始到结束标记之间的内容
                content = content[:start_idx] + content[end_idx + 1:]

    return content

def clean_unused_imports(content):
    """步骤 4: 清理未使用的类型导入"""

    # 移除未使用的类型
    unused_types = [
        'ContextCompressionCandidate, ',
        'ContextEvidence, ',
        'ContextSection, ',
        'ExecutionSection, ',
        'LLMContextBudgetTrace, ',
        'LLMUsageProviderModelBreakdown, ',
        'LLMUsageSummary, ',
        'LLMUsageTrace, ',
        'MemoryRecallTrace, ',
        'ProgressBlocker, ',
        'ProgressItem, ',
        'RepairLoop, ',
        'RuntimeCapability, ',
        'TaskDecomposition, ',
        'TaskDecompositionNode, ',
    ]

    for unused_type in unused_types:
        content = content.replace(unused_type, '')

    return content

def main():
    file_path = 'src/main.tsx'

    print("📖 读取文件...")
    content = read_file(file_path)
    original_lines = len(content.split('\n'))

    print("✏️  步骤 1: 添加组件导入...")
    content = add_imports(content)

    print("✏️  步骤 2: 替换函数调用...")
    content = replace_function_calls(content)

    print("✏️  步骤 3: 删除旧函数定义...")
    content = remove_function_definitions(content)

    print("✏️  步骤 4: 清理未使用的类型导入...")
    content = clean_unused_imports(content)

    print("💾 写入文件...")
    write_file(file_path, content)

    new_lines = len(content.split('\n'))
    print(f"\n✅ 集成完成!")
    print(f"📊 原始行数: {original_lines}")
    print(f"📊 新行数: {new_lines}")
    print(f"📊 减少: {original_lines - new_lines} 行")
    print(f"📊 减少比例: {((original_lines - new_lines) / original_lines * 100):.1f}%")

if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        print(f"❌ 错误: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
