import {useEffect, useState} from 'react';
import {fetchHealth, saveCurseForgeKey, type SystemHealth} from '../api/system';
import {useOnboarding} from '../app/OnboardingContext';

/* 素面叶子：设置页。第三步换成 SettingsPanel（目录选择 + Prism 工具 + 完整自检）。
   令牌只显示已配置/未配置，永不显示值（铁律：令牌不落盘、不外显）。 */
export function StubSettings() {
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [key, setKey] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {ack, reload} = useOnboarding();

  const load = () => fetchHealth().then(setHealth).catch(e => setError(e instanceof Error ? e.message : String(e)));
  useEffect(() => { void load(); }, []);

  /* Key 真实存下来之后才打勾（§5.8：ack 的触发点是动作真实完成）。 */
  const save = async () => {
    await saveCurseForgeKey(key.trim());
    setKey('');
    await load();
    await ack('curseforgeKey');
    await reload();
  };

  const tokenSet = __MPACK_WRITE_TOKEN__.length > 0;

  return (
    <div>
      {error && <div className="rr-line">{error}</div>}
      <div className="rr-line">写令牌：{tokenSet ? '已配置' : '未配置'}（值不显示）</div>
      {health && (
        <div className="rr-line">
          CurseForge Key {health.curseforgeKeyConfigured ? '已配置' : '未配置'} ·
          CurseForge {health.curseforgeReachable ? '可达' : '不可达'} · Modrinth {health.modrinthReachable ? '可达' : '不可达'} ·
          存储可写 {health.storageWritable ? '是' : '否'} · 剩余 {(health.storageFreeBytes / 1024 / 1024).toFixed(0)} MB
        </div>
      )}
      <input className="tb-btn" value={key} onChange={e => setKey(e.target.value)} placeholder="CurseForge API Key"/>
      <button type="button" className="tb-btn" onClick={() => void save().catch(e => setError(String(e)))}>保存 Key</button>
    </div>
  );
}
