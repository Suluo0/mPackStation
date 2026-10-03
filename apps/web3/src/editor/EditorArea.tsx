import {useUrlState} from '../app/url';
import {EditorHeader} from './EditorHeader';
import {ModeIndex} from './ModeIndex';
import {ModeGraph} from './ModeGraph';
import {ModeEdit} from './ModeEdit';
import {ModeQuest} from './ModeQuest';
import {Welcome} from './Welcome';
import './editor.css';

/* 编辑区（V3 §1 右侧主体）：无包 = 迎新；有包 = 编辑区头（选中对象才出现，含四透镜）
   + 当前态画布。四个态组件零 props、自己读 URL（第三步替换叶子时容器不动）。 */
export function EditorArea() {
  const {packId, mode} = useUrlState();

  if (!packId) return <Welcome/>;

  return (
    <main className="editor">
      <EditorHeader/>
      {mode === 'index' && <ModeIndex/>}
      {mode === 'graph' && <ModeGraph/>}
      {mode === 'edit' && <ModeEdit/>}
      {mode === 'quest' && <ModeQuest/>}
    </main>
  );
}
