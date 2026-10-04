import {useEffect, useState} from 'react';
import {Modal} from 'antd';
import {fetchHealth, saveCurseForgeKey, clearCurseForgeKey, type SystemHealth} from '../api/system';
import {useOnboarding} from './OnboardingContext';
import {useUrlPatch} from './url';
import {type ThemeName} from './theme';
import {useTheme} from './ThemeContext';

/* 设置弹窗（⚙ / ?settings=1）。令牌只显示已配置/未配置，永不显示值。 */
const THEME_OPTS: {value: ThemeName | null; label: string}[] = [
  {value: null, label: '跟随系统'},
  {value: 'light', label: '浅色'},
  {value: 'dark', label: '深色'},
];
export function SettingsModal({open}: {open: boolean}) {
  const patch = useUrlPatch();
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [key, setKey] = useState('');
  const [error, setError] = useState<string | null>(null);
  /* 主题走 ThemeProvider，不是直接 applyTheme —— 后者的 antd 控件不会跟着变。 */
  const {preference, setTheme} = useTheme();
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

  return (
    <Modal open={open} onCancel={() => patch({settings: null})} footer={null} title="设置" width={560}>
      {error && <div className="p-empty">{error}</div>}
      {/* 主题：只切 data-theme，色值全在 styles/themes.css，这里不存任何颜色 */}
      <div style={{display: 'flex', alignItems: 'center', gap: 6, fontSize: 13, marginBottom: 10}}>
        <span style={{width: 96, color: 'var(--mc-text-2)'}}>主题</span>
        {THEME_OPTS.map(o => (
          <button key={o.label} type="button" className={`p-btn${preference === o.value ? ' primary' : ''}`}
            onClick={() => setTheme(o.value)}>{o.label}</button>
        ))}
      </div>
      {health && (
        <div style={{display: 'flex', flexDirection: 'column', gap: 6, fontSize: 13}}>
          <div>CurseForge Key {health.curseforgeKeyConfigured ? '✓ 已配置' : '未配置'}</div>
          <div>CurseForge {health.curseforgeReachable ? '✓ 可达' : '✖ 不可达'} · Modrinth {health.modrinthReachable ? '✓ 可达' : '✖ 不可达'}</div>
          <div>存储可写 {health.storageWritable ? '是' : '否'} · 剩余 {(health.storageFreeBytes / 1024 / 1024).toFixed(0)} MB</div>
        </div>
      )}
      <div style={{display: 'flex', gap: 6, marginTop: 12}}>
        <input className="p-input" style={{flex: 1}} value={key} placeholder="CurseForge API Key"
          onChange={e => setKey(e.target.value)}/>
        <button type="button" className="p-btn primary" disabled={!key.trim()} onClick={() => void save().catch(e => setError(String(e)))}>保存 Key</button>
        {health?.curseforgeKeyConfigured && (
          <button type="button" className="p-btn" onClick={() => void clearCurseForgeKey().then(() => load()).catch((e: Error) => setError(String(e)))}>清除 Key</button>
        )}
      </div>
    </Modal>
  );
}
