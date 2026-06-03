# 决策阶段

当前执行路径遇到阻塞，需要人工决策。

## 允许的决策类型
- provide_missing_info：需要补充信息以继续执行
- replan：当前路径不可行，需要重新规划

## 禁止
- 不要使用 reject_current_path 直接终止
- 不要在 decision 中尝试继续执行

## 决策格式
- reason: 清晰描述为什么需要决策
- question: 具体的问题
- context: 提供足够上下文帮助决策者
