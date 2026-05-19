#!/bin/bash

echo "=== 页面组件分析 ==="
echo ""

# 定义页面组件及其起始行
declare -A pages=(
    ["WorkspaceBoardPage"]=2567
    ["DecisionQueuePage"]=2693
    ["ApprovalQueuePage"]=2941
    ["GovernanceQueuePage"]=3134
    ["WorkspaceDetailPage"]=3158
    ["RuntimeControlPage"]=5781
    ["ConfigurationPage"]=6353
    ["ServicePage"]=7127
)

# 获取文件总行数
total_lines=$(wc -l < src/main.tsx)

# 按行号排序
sorted_pages=($(for page in "${!pages[@]}"; do
    echo "${pages[$page]} $page"
done | sort -n))

# 计算每个页面的行数
prev_line=0
prev_name=""

for i in "${!sorted_pages[@]}"; do
    line_info="${sorted_pages[$i]}"
    start_line=$(echo $line_info | cut -d' ' -f1)
    page_name=$(echo $line_info | cut -d' ' -f2)
    
    if [ $prev_line -ne 0 ]; then
        lines=$((start_line - prev_line))
        echo "$prev_name: 第 $prev_line 行开始, 约 $lines 行"
    fi
    
    prev_line=$start_line
    prev_name=$page_name
done

# 最后一个页面
if [ $prev_line -ne 0 ]; then
    lines=$((total_lines - prev_line))
    echo "$prev_name: 第 $prev_line 行开始, 约 $lines 行"
fi

echo ""
echo "总行数: $total_lines"
