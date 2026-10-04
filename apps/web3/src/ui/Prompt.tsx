import {useState} from 'react';
import {Modal} from './Modal';
import {Button} from './Button';
import {TextInput} from './TextInput';

/* 单输入弹窗：新建分类 / 重命名 / 填名字这类「问一句话」的场景全走它。
   以前每个面板自己存一份 prompt state + 一份 JSX，Esc、空值、聚焦
   三件事各写各的 —— 有的漏了 Esc，有的空值也能提交。 */
export function Prompt({title, okLabel = '确定', placeholder, initial = '', onOk, onClose}: {
  title: string;
  okLabel?: string;
  placeholder?: string;
  initial?: string;
  onOk: (value: string) => void;
  onClose: () => void;
}) {
  const [value, setValue] = useState(initial);
  /* 空值不许提交：分类名 / 名字这类字段空串在后端没有落脚点，
     提交上去只会得到一个「看起来没反应」的失败。 */
  const ok = () => { const v = value.trim(); if (v) onOk(v); onClose(); };
  return (
    <Modal onClose={onClose}>
      <div className="p-title">{title}</div>
      <TextInput value={value} onChange={setValue} placeholder={placeholder} autoFocus
        onEnter={ok} ariaLabel={title}/>
      <div className="mm-actions">
        <Button onClick={onClose}>取消</Button>
        <Button variant="primary" disabled={!value.trim()} onClick={ok}>{okLabel}</Button>
      </div>
    </Modal>
  );
}
