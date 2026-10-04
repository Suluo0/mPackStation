import type {ReactNode} from 'react';

/* 只读详情的「字段 → 值」栅格。

   日志、模组、版本、任务这些详情本质是一张键值表：标签 + 值 + 偶尔要等宽字体
   （id、路径、耗时）。以前每处手写一行 label + 一行 value，对齐各写各的。

   只做**只读**。可编辑表单（任务节点检查器、配方编辑）不进这里 ——
   编辑要管脏状态、校验、保存，那是另一套东西，混进来这个组件会立刻长歪。 */
export type DetailField = {
  label: string;
  value?: ReactNode;
  /** 等宽字体：id、路径、时间戳这类要逐字符对齐的值。 */
  mono?: boolean;
  /** 占满整行：错误信息、长描述。 */
  wide?: boolean;
  /** 值用语义色（失败红 / 成功绿）。 */
  tone?: 'fail' | 'ok' | 'muted';
};

export function DetailFields({fields, columns = 2}: {fields: DetailField[]; columns?: 1 | 2 | 3}) {
  const shown = fields.filter(f => f.value != null && f.value !== '');
  if (shown.length === 0) return null;
  return (
    <div className="df" style={{gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`}}>
      {shown.map(f => (
        <div key={f.label} className={`df-item${f.wide ? ' wide' : ''}`}>
          <span className="df-label">{f.label}</span>
          <span className={`df-value${f.mono ? ' mono' : ''}${f.tone ? ` tone-${f.tone}` : ''}`}>{f.value}</span>
        </div>
      ))}
    </div>
  );
}
