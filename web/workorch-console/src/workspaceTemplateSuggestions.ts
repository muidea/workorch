// 模板只为尚未填写的字段提供建议，不取得用户创作内容的所有权。
export function workspaceTemplateSuggestions(
  current: { goalText?: string; doDText?: string; goalTouched: boolean; doDTouched: boolean },
  template: { goalText?: string; doDItems?: string[] }
): { goalText?: string; doDText?: string } {
  const suggestions: { goalText?: string; doDText?: string } = {};
  if (!current.goalTouched && !current.goalText?.trim() && template.goalText?.trim()) {
    suggestions.goalText = template.goalText;
  }
  if (!current.doDTouched && !current.doDText?.trim() && template.doDItems?.length) {
    suggestions.doDText = template.doDItems.join('\n');
  }
  return suggestions;
}
