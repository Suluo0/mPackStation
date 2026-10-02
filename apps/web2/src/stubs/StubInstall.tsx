import {useEffect, useState} from 'react';
import {listLauncherInstalls, type LauncherInstall} from '../api/launcher';

/* 素面叶子：交付③安装。第三步换成 InstallSection（选游戏目录 + 装版本 + 任务进度）。
   这里只列后端已登记的真实安装记录，不提供没有目录选择的安装按钮。 */
export function StubInstall({packId}: {packId: string}) {
  const [installs, setInstalls] = useState<LauncherInstall[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listLauncherInstalls(packId).then(setInstalls).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      <div className="rr-line">已登记安装 {installs.length}</div>
      {installs.map(i => (
        <div key={i.id} className="rr-line">
          {i.versionId} · {i.loader} · MC {i.mcVersion} · {i.minecraftDir}
        </div>
      ))}
      {installs.length === 0 && !error && <div className="rr-line">这个包还没有装进任何游戏目录。构建出产物后即可安装。</div>}
    </div>
  );
}
