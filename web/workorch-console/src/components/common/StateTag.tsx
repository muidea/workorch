import { Tag } from 'antd';
import { stateLabel, getStateColor } from '../../utils/format';

interface StateTagProps {
  state?: string;
}

/**
 * 状态标签组件
 * 根据状态显示不同颜色的标签
 */
export function StateTag({ state }: StateTagProps) {
  const label = stateLabel(state);
  const color = getStateColor(state);

  return <Tag color={color}>{label}</Tag>;
}
