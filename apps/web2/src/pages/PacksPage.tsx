import {useEffect, useState} from 'react';
import {Button, Input, Select} from 'antd';
import {createPack, loaderEnum} from '../api/packs';
import {confirmImport, inspectImport, type ImportPreview, type ImportSource} from '../api/imports';
import {fetchMcVersions} from '../api/system';
import {useOnboarding} from '../app/OnboardingContext';
import {StubPackList} from '../stubs/StubPackList';

const LOADERS = loaderEnum.options;
const SOURCES: {value: ImportSource; label: string}[] = [
  {value: 'curseforge', label: 'CurseForge 链接'},
  {value: 'modrinth', label: 'Modrinth 链接'},
];

/** 建包 / 导包成功后打勾迎新位：动作真实发生，不是点了跳转就算（§5.8）。 */
function useAfterPackCreated() {
  const {ack} = useOnboarding();
  return () => void ack('firstPack').catch(() => undefined);
}

export function CreatePackForm() {
  const [name, setName] = useState('');
  const [mc, setMc] = useState<string>('');
  const [loader, setLoader] = useState<string>('fabric');
  const [mcVersions, setMcVersions] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const created = useAfterPackCreated();
  useEffect(() => { fetchMcVersions().then(setMcVersions).catch(() => setMcVersions([])); }, []);

  const submit = async () => {
    setBusy(true); setError(null);
    try {
      await createPack({name: name.trim(), mcVersion: mc, loader});
      setName('');
      created();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center'}}>
      <Input placeholder="包名" value={name} onChange={e => setName(e.target.value)} style={{width: 160}}/>
      <Select placeholder="MC 版本" value={mc || undefined} onChange={setMc} style={{width: 140}}
        options={(mcVersions.length ? mcVersions : ['1.21.1']).map(v => ({value: v, label: v}))}/>
      <Select value={loader} onChange={setLoader} style={{width: 120}} options={LOADERS.map(l => ({value: l, label: l}))}/>
      <Button type="primary" loading={busy} disabled={!name.trim() || !mc} onClick={() => void submit()}>新建整合包</Button>
      {error && <span className="rr-line">{error}</span>}
    </div>
  );
}

export function ImportPackForm() {
  const [source, setSource] = useState<ImportSource>('modrinth');
  const [url, setUrl] = useState('');
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const created = useAfterPackCreated();

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError(null);
    try { await fn(); } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };

  return (
    <div style={{display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center'}}>
      <Select value={source} onChange={setSource} style={{width: 150}} options={SOURCES}/>
      <Input placeholder="包链接" value={url} onChange={e => setUrl(e.target.value)} style={{width: 260}}/>
      <Button loading={busy} disabled={!url.trim()} onClick={() => act(async () => setPreview(await inspectImport({source, url: url.trim()})))}>解析</Button>
      {preview && (
        <>
          <span className="rr-line">{preview.packName || '(未命名)'} · {preview.entryCount} 个条目</span>
          <Button type="primary" loading={busy} disabled={!url.trim()}
            onClick={() => act(async () => { await confirmImport(preview, crypto.randomUUID()); setPreview(null); setUrl(''); created(); })}>确认导入</Button>
        </>
      )}
      {error && <span className="rr-line">{error}</span>}
    </div>
  );
}

/* /packs：全部包的表格 + 新建/导入入口（§6.2）。 */
export function PacksPage() {
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      <div style={{display: 'flex', gap: 12, flexWrap: 'wrap'}}>
        <CreatePackForm/>
        <ImportPackForm/>
      </div>
      <StubPackList/>
    </div>
  );
}
