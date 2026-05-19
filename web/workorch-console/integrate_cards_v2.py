#!/usr/bin/env python3
"""
手动集成卡片组件 - 分步骤执行
"""

import sys

def read_file(path):
    with open(path, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(path, content):
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

def step1_add_imports(content):
    """添加组件导入"""
    import_statement = """
import {
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

    # 在 ProCard 导入之后插入
    lines = content.split('\n')
    for i, line in enumerate(lines):
        if "from '@ant-design/pro-components'" in line:
            lines.insert(i + 1, import_statement)
            break

    return '\n'.join(lines)

def step2_replace_calls(content):
    """替换函数调用为组件调用"""

    # 简单的字符串替换，避免正则表达式的复杂性
    replacements = [
        # 在 JSX 中的调用
        ('{taskProgressCard(runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress, runObservability?.repairLoop ?? runSnapshot?.repairLoop)}',
         '<TaskProgressCard progress={runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress} repairLoopOverride={runObservability?.repairLoop ?? runSnapshot?.repairLoop} />'),

        ('{taskDecompositionCard(runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions)}',
         '<TaskDecompositionCard items={runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions} />'),

        ('{executionSectionsCard(runSnapshot?.executionSections)}',
         '<ExecutionSectionsCard items={runSnapshot?.executionSections} />'),

        ('{memoryRecallCard(runSnapshot.memoryRecallTraces)}',
         '<MemoryRecallCard items={runSnapshot.memoryRecallTraces} />'),

        ('{webSourceCard(runWebSources)}',
         '<WebSourceCard items={runWebSources} />'),

        ('{llmContextBudgetCard(runLLMContextBudgets)}',
         '<LLMContextBudgetCard traces={runLLMContextBudgets} />'),

        ('{llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)}',
         '<LLMUsageCard summary={runLLMUsageSummary} traces={runLLMUsageTraces} />'),

        ('{contextBundleCard(contextBundle)}',
         '<ContextBundleCard bundle={contextBundle} />'),

        # 在 Tabs items 中的调用
        ('children: webSourceCard(runWebSources)',
         'children: <WebSourceCard items={runWebSources} />'),

        ('children: llmContextBudgetCard(runLLMContextBudgets)',
         'children: <LLMContextBudgetCard traces={runLLMContextBudgets} />'),

        ('children: llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)',
         'children: <LLMUsageCard summary={runLLMUsageSummary} traces={runLLMUsageTraces} />'),
    ]

    for old, new in replacements:
        content = content.replace(old, new)

    return content

def step3_remove_functions(content):
    """删除旧函数定义"""
    lines = content.split('\n')
    result = []
    skip_until_blank = False
    function_depth = 0
    in_function = None

    functions_to_remove = {
        'taskProgressCard',
        'taskDecompositionCard',
        'executionSectionsCard',
        'memoryRecallCard',
        'webSourceCard',
        'llmUsageCard',
        'llmContextBudgetCard',
        'contextBundleCard'
    }

    i = 0
    while i < len(lines):
        line = lines[i]

        # 检查是否是要删除的函数定义
        if not in_function:
            for func_name in functions_to_remove:
                if line.startswith(f'function {func_name}('):
                    in_function = func_name
                    function_depth = 0
                    break

        if in_function:
            # 计算大括号深度
            function_depth += line.count('{') - line.count('}')

            # 如果深度回到 0，说明函数结束
            if function_depth == 0 and ('{' in line or '}' in line):
                in_function = None
                # 跳过函数后的空行
                i += 1
                while i < len(lines) and lines[i].strip() == '':
                    i += 1
                continue
        else:
            result.append(line)

        i += 1

    return '\n'.join(result)

def main():
    file_path = 'src/main.tsx'

    print("📖 读取文件...")
    content = read_file(file_path)
    original_lines = len(content.split('\n'))

    print("✏️  步骤 1: 添加组件导入...")
    content = step1_add_imports(content)

    print("✏️  步骤 2: 替换函数调用...")
    content = step2_replace_calls(content)

    print("✏️  步骤 3: 删除旧函数定义...")
    content = step3_remove_functions(content)

    print("💾 写入文件...")
    write_file(file_path, content)

    new_lines = len(content.split('\n'))
    print(f"\n✅ 集成完成!")
    print(f"📊 原始行数: {original_lines}")
    print(f"📊 新行数: {new_lines}")
    print(f"📊 减少: {original_lines - new_lines} 行")

if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        print(f"❌ 错误: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
