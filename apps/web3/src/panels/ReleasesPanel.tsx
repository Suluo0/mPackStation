import {useCallback, useEffect, useState} from 'react';
import {createVersion, listVersions, type PackVersion} from '../api/releases';
import {useUrlState} from '../app/url';
import {Button} from '../ui/Button';
import {Prompt} from '../ui/Prompt';

/* 包版本修订（左侧第 4 个按钮 Get Version）。

   这里**只管版本**。构建和运行不是「版本下面的动作」，是独立的按钮
   （顶边栏右侧），曾经把它们塞进这个面板的「交付动作」段，等于把一级能力
   降成了二级入口 —— 已拆走，别再加回来。 */
export function ReleasesPanel() {
  const {packId} = useUrlState();
  const [versions, setVersions] = useState<PackVersion[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const load = useCallback(() => {
    if (!packId) return;
    listVersions(packId)
      .then(setVersions)
      .catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);
  useEffect(load, [load]);

  if (!packId) return null;

  return (
    <div className="tp-body">
      <div className="p-section">
        <div className="p-title">包版本 <span className="count">{versions.length}</span></div>
        {error && <div className="p-empty">{error}</div>}
        {versions.length === 0 && !error && (
          <div className="p-empty">还没有注册过版本。构建 / 发布都以包版本为准，先建一个。</div>
        )}
        {versions.map(v => (
          <div key={v.id} className="p-row">
            <span className="grow">{v.version}</span>
            <span className="sub">{v.channel}</span>
          </div>
        ))}
        <Button icon="plus" onClick={() => setCreating(true)}>新建版本…</Button>
      </div>

      {creating && (
        <Prompt title="新版本号" okLabel="创建" placeholder="如 0.2.0"
          onOk={v => {
            void createVersion(packId, {version: v, channel: 'release'})
              .then(load)
              .catch(e => setError(e instanceof Error ? e.message : String(e)));
          }}
          onClose={() => setCreating(false)}/>
      )}
    </div>
  );
}
