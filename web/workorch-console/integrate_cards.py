#!/usr/bin/env python3
"""
自动集成卡片组件到 main.tsx
"""

import re
import sys

def integrate_cards(file_path):
    with open(file_path, 'r', encoding='utf-8') as f:
        content = f.read()

    original_content = content

    # Step 1: 添加组件导入（在现有导入之后）
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

    # 在第一个 import 之后插入
    import_pattern = r"(import.*?from '@ant-design/pro-components';)"
    content = re.sub(import_pattern, r"\1\n" + import_statement, content, count=1)

    # Step 2: 替换函数调用为组件调用
    replacements = [
        # TaskProgressCard
        (
            r'\{taskProgressCard\((.*?), (.*?)\)\}',
            r'<TaskProgressCard progress={\1} repairLoopOverride={\2} />'
        ),
        # TaskDecompositionCard
        (
            r'\{taskDecompositionCard\((.*?)\)\}',
            r'<TaskDecompositionCard items={\1} />'
        ),
        # ExecutionSectionsCard
        (
            r'\{executionSectionsCard\((.*?)\)\}',
            r'<ExecutionSectionsCard items={\1} />'
        ),
        # MemoryRecallCard
        (
            r'\{memoryRecallCard\((.*?)\)\}',
            r'<MemoryRecallCard items={\1} />'
        ),
        # WebSourceCard
        (
            r'webSourceCard\((.*?)\)',
            r'<WebSourceCard items={\1} />'
        ),
        # LLMUsageCard
        (
            r'llmUsageCard\((.*?), (.*?)\)',
            r'<LLMUsageCard summary={\1} traces={\2} />'
        ),
        # LLMContextBudgetCard
        (
            r'llmContextBudgetCard\((.*?)\)',
            r'<LLMContextBudgetCard traces={\1} />'
        ),
        # ContextBundleCard
        (
            r'\{contextBundleCard\((.*?)\)\}',
            r'<ContextBundleCard bundle={\1} />'
        ),
    ]

    for pattern, replacement in replacements:
        content = re.sub(pattern, replacement, content)

    # Step 3: 删除旧函数定义
    functions_to_remove = [
        'taskProgressCard',
        'taskDecompositionCard',
        'executionSectionsCard',
        'memoryRecallCard',
        'webSourceCard',
        'llmUsageCard',
        'llmContextBudgetCard',
        'contextBundleCard'
    ]

    for func_name in functions_to_remove:
        # 匹配函数定义（从 function 开始到下一个顶层函数或文件末尾）
        pattern = rf'^function {func_name}\(.*?\n(?:.*?\n)*?\}}\n\n'
        content = re.sub(pattern, '', content, flags=re.MULTILINE)

    # 如果内容有变化，写回文件
    if content != original_content:
        with open(file_path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"✅ 成功集成卡片组件到 {file_path}")
        print(f"📊 原始行数: {len(original_content.splitlines())}")
        print(f"📊 新行数: {len(content.splitlines())}")
        print(f"📊 减少: {len(original_content.splitlines()) - len(content.splitlines())} 行")
        return True
    else:
        print("⚠️  没有检测到需要修改的内容")
        return False

if __name__ == '__main__':
    file_path = 'src/main.tsx'
    if len(sys.argv) > 1:
        file_path = sys.argv[1]

    try:
        integrate_cards(file_path)
    except Exception as e:
        print(f"❌ 错误: {e}")
        sys.exit(1)
