import {useEffect, useState} from 'react';
import {Modal} from 'antd';
import {fetchHealth, saveCurseForgeKey, type SystemHealth} from '../api/system';
import {useOnboarding} from './OnboardingContext';
import {useUrlPatch} from './url';

/* 设置弹窗（⚙ / ?settings=1）。令牌只显示已配置/未配置，永不显示值。 */
export function SettingsModal({open}: {open: boolean}) {
  const patch = useUrlPatch();
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [key, setKey] = useState('');
  const [error, setError] = useState<string | null>(null);
  const {ack, reload} = useOnboarding();

  const load = () => fetchHealth().then(setHealth).catch(e => setError(e instanceof Error ? e.message : String(e)));
  useEffect(() => { if (open) void load(); }, [open]);

  const save = async () => {
    await saveCurseForgeKey(key.trim());
    setKey('');
    await load();
    await ack('curseforgeKey');
    await reload();
  };

  const tokenSet = __MPACK_WRITE_TOKEN__.length > 0;

  return (
    <Modal open={open} onCancel={() => patch({settings: null})} footer={null} title="设置" width={560}>
      {error && <div className="p-empty">{error}</div>}
      {health && (
        <div style={{display: 'flex', flexDirection: 'column', gap: 6, fontSize: 13}}>
          <div>CurseForge Key {health.curseforgeKeyConfigured ? '✓ 已配置' : '未配置'}</div>
          <div>CurseForge {health.curseforgeReachable ? '✓ 可达' : '✖ 不可达'} · Modrinth {health.modrinthReachable ? '✓ 可达' : '✖ 不可达'}</div>
          <div>存储可写 {health.storageWritable ? '是' : '否'} · 剩余 {(health.storageFreeBytes / 1024 / 1024).toFixed(0)} MB</div>
          <div>写令牌 {tokenSet ? '✓ 已配置' : '✖ 未配置'}（值不显示）</div>
        </div>
      )}
      <div style={{display: 'flex', gap: 6, marginTop: 12}}>
        <input className="p-input" style={{flex: 1}} value={key} placeholder="CurseForge API Key"
          onChange={e => setKey(e.target.value)}/>
        <button type="button" className="p-btn primary" disabled={!key.trim()} onClick={() => void save().catch(e => setError(String(e)))}>保存 Key</button>
      </div>
    </Modal>
  );
}
