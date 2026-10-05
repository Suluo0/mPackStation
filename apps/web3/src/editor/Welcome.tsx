import {Button, Input, Select} from 'antd';
import {useEffect, useState} from 'react';
import {createPack, loaderEnum} from '../api/packs';
import {confirmImport, inspectImport, type ImportPreview, type ImportSource} from '../api/imports';
import {fetchMcVersions} from '../api/system';
import {CHECKLIST, useOnboarding} from '../app/OnboardingContext';
import {useUrlPatch} from '../app/url';

/* 迎新（无包态，V3 §2）：占据整个编辑区。四步与侧栏清单同源（useOnboarding），
   建包/导包成功后打勾并自动切进新包。表单迁自 web2 PacksPage（零改动逻辑）。 */
const LOADERS = loaderEnum.options;
const SOURCES: {value: ImportSource; label: string}[] = [
  {value: 'modrinth', label: 'Modrinth 链接'},
  {value: 'curseforge', label: 'CurseForge 链接'},
  {value: 'local', label: '本地 zip / mrpack'},
];

export function Welcome() {
  const {steps} = useOnboarding();
  const patch = useUrlPatch();

  return (
    <div className="welcome">
      <div className="wl-hero">
        <h1>◆ mPackStation</h1>
        <p>从搜索到打包，不启动游戏，完成你的整合包。先建一个新包，或从 CurseForge / Modrinth 导入现成的包。</p>
      </div>

      <div className="wl-card">
        <div className="wl-title">新建整合包</div>
        <CreatePackForm onCreated={id => patch({pack: id, mode: null, tool: 'focus'}, {push: true})}/>
      </div>

      <div className="wl-card">
        <div className="wl-title">导入整合包</div>
        <ImportPackForm onImported={id => patch({pack: id, mode: null, tool: 'focus'}, {push: true})}/>
      </div>

      <div className="wl-steps">
        {CHECKLIST.map(s => {
          const done = !!steps?.[s.key];
          return (
            <div key={s.key} className={`wl-step${done ? ' done' : ' next'}`}>
              <span>{done ? '✓' : '○'}</span> {s.label}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function useAfterPackCreated() {
  const {ack} = useOnboarding();
  return () => void ack('firstPack').catch(() => undefined);
}

function CreatePackForm({onCreated}: {onCreated: (id: string) => void}) {
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
      const pack = await createPack({name: name.trim(), mcVersion: mc, loader});
      setName('');
      created();
      onCreated(pack.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center'}}>
      <Input placeholder="包名" value={name} onChange={e => setName(e.target.value)} style={{width: 180}}/>
      <Select placeholder="MC 版本" value={mc || undefined} onChange={setMc} style={{width: 140}}
        options={(mcVersions.length ? mcVersions : ['1.21.1']).map(v => ({value: v, label: v}))}/>
      <Select value={loader} onChange={setLoader} style={{width: 120}} options={LOADERS.map(l => ({value: l, label: l}))}/>
      <Button type="primary" loading={busy} disabled={!name.trim() || !mc} onClick={() => void submit()}>新建整合包</Button>
      {error && <span className="p-empty" style={{padding: 0}}>{error}</span>}
    </div>
  );
}

/* 本地文件 → base64（导入链路的 local_zip 来源从一开始就要的就是这个形态）。 */
const readAsBase64 = (f: File) =>
  new Promise<string>((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).split(',')[1] ?? '');
    r.onerror = () => reject(new Error('本地文件读取失败'));
    r.readAsDataURL(f);
  });

function ImportPackForm({onImported}: {onImported: (id: string) => void}) {
  const [source, setSource] = useState<ImportSource>('modrinth');
  const [url, setUrl] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [drag, setDrag] = useState(false);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  /* 确认导入只返回任务（packId 要等后台任务跑完才有），这里必须给一句话落点，
     否则用户点完「确认导入」界面毫无变化，以为按钮没生效。 */
  const [notice, setNotice] = useState<string | null>(null);
  const created = useAfterPackCreated();

  const act = async (fn: () => Promise<void>) => {
    setBusy(true); setError(null);
    try { await fn(); } catch (e) { setError(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };

  return (
    <div style={{display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center'}}>
      <Select value={source} onChange={setSource} style={{width: 150}} options={SOURCES}/>
      {source === 'local' ? (
        <label className={`wl-dropzone${drag ? ' over' : ''}`}
          onDragOver={e => { e.preventDefault(); setDrag(true); }}
          onDragLeave={() => setDrag(false)}
          onDrop={e => {
            e.preventDefault(); setDrag(false);
            const f = e.dataTransfer.files?.[0];
            if (f) { setFile(f); setPreview(null); }
          }}>
          <input type="file" accept=".zip,.mrpack"
            onChange={e => { setFile(e.target.files?.[0] ?? null); setPreview(null); }}/>
          {file
            ? <span style={{color: 'var(--mc-text)'}}>{file.name}（{Math.max(1, Math.round(file.size / 1024))} KB）— 点击可重选</span>
            : '把 zip / mrpack 拖到这里，或点击选择文件'}
        </label>
      ) : (
        <Input placeholder="包链接" value={url} onChange={e => setUrl(e.target.value)} style={{width: 280}}/>
      )}
      <Button loading={busy}
        disabled={source === 'local' ? !file : !url.trim()}
        onClick={() => act(async () => {
          setNotice(null);
          setPreview(source === 'local' && file
            ? await inspectImport({source: 'local', contentBase64: await readAsBase64(file)})
            : await inspectImport({source, url: url.trim()}));
        })}>解析</Button>
      {preview && (
        <>
          <span className="sub">{preview.packName || '(未命名)'} · {preview.entryCount} 个条目</span>
          <Button type="primary" loading={busy} disabled={source === 'local' ? !file : !url.trim()}
            onClick={() => act(async () => {
              const done = await confirmImport(preview, crypto.randomUUID());
              setPreview(null); setUrl(''); created();
              if (done.packId) onImported(done.packId); // 导入是后台任务，packId 就绪才切
              else setNotice(`导入任务已提交（${done.taskId}${done.reused ? '，复用此前同一任务' : ''}），进度见底部「任务」，完成后包自动出现在列表。`);
            })}>确认导入</Button>
        </>
      )}
      {notice && <span className="sub">{notice}</span>}
      {error && <span className="p-empty" style={{padding: 0}}>{error}</span>}
    </div>
  );
}
