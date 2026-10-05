import {Button, Input, Select} from 'antd';
import {useEffect, useState} from 'react';
import {createPack, loaderEnum} from '../api/packs';
import {browseDirectories, type FsBrowse} from '../api/fs';
import {Modal} from '../ui/Modal';
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


const isImportableName = (name: string) => {
  const n = name.toLowerCase();
  return n.endsWith('.zip') || n.endsWith('.mrpack');
};

function ImportPackForm({onImported}: {onImported: (id: string) => void}) {
  const [source, setSource] = useState<ImportSource>('modrinth');
  const [url, setUrl] = useState('');
  const [filePath, setFilePath] = useState('');
  const [drag, setDrag] = useState(0);
  const [browsing, setBrowsing] = useState(false);

  /* 全页拖放（2026-10-05）：拖任何文件进来都出现虚线框。流程分两段——
     类型不对直接拒绝（「这个文件类型不支持」）；类型对 → 立即做元数据检查
     （inspect），出预览后才给「确认导入」。 */
  useEffect(() => {
    const hasFiles = (e: DragEvent) => Array.from(e.dataTransfer?.types ?? []).includes('Files');
    const enter = (e: DragEvent) => { if (hasFiles(e)) { e.preventDefault(); setDrag(c => c + 1); } };
    const leave = (e: DragEvent) => { if (hasFiles(e) && !e.relatedTarget) setDrag(0); };
    const over = (e: DragEvent) => { if (hasFiles(e)) e.preventDefault(); };
    const drop = (e: DragEvent) => { setDrag(0); void handleDropFile(e.dataTransfer?.files?.[0]); };
    document.addEventListener('dragenter', enter);
    document.addEventListener('dragleave', leave);
    document.addEventListener('dragover', over);
    document.addEventListener('drop', drop);
    return () => {
      document.removeEventListener('dragenter', enter);
      document.removeEventListener('dragleave', leave);
      document.removeEventListener('dragover', over);
      document.removeEventListener('drop', drop);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
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

  /* 前端只做格式校验 + 把文件地址发后端（2026-10-05 定稿：内容不走前端）。
     桌面应用形态下 File 自带 path；浏览器里拿不到 path，拖拽会提示改用浏览本机。 */
  const handleFilePath = async (path?: string) => {
    if (!path) return;
    if (!isImportableName(path)) {
      setError('这个文件类型不支持 —— 只接受 .zip / .mrpack');
      return;
    }
    setSource('local'); setFilePath(path); setPreview(null); setError(null);
    await act(async () => { setPreview(await inspectImport({source: 'local', path})); });
  };

  const handleDropFile = (f?: File | null) => {
    if (!f) return;
    if (!isImportableName(f.name)) {
      setError('这个文件类型不支持 —— 只接受 .zip / .mrpack');
      return;
    }
    const p = (f as File & {path?: string}).path;
    if (p) { void handleFilePath(p); return; }
    setError('浏览器里拖拽拿不到文件路径 —— 点「浏览本机文件」选择，打包为应用后拖拽直传');
    setBrowsing(true);
  };

  return (
    <>
      {drag > 0 && (
        <div className="wl-drag-overlay" onDragOver={e => e.preventDefault()}>
          <div className="wl-drag-box">支持直接拖动 mrpack 或 zip 包到这里自动导入识别</div>
        </div>
      )}
    <div style={{display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center'}}>
      <Select value={source} onChange={setSource} style={{width: 150}} options={SOURCES}/>
      {source === 'local' ? (
        <>
          <label className={`wl-dropzone${drag > 0 ? ' over' : ''}`}
            onDragOver={e => { e.preventDefault(); setDrag(1); }}
            onDragLeave={() => setDrag(0)}
            onDrop={e => { e.preventDefault(); setDrag(0); handleDropFile(e.dataTransfer?.files?.[0]); }}>
            <input type="file" accept=".zip,.mrpack"
              onChange={e => {
                const f = e.target.files?.[0];
                if (f) {
                  const p = (f as File & {path?: string}).path;
                  if (p) { void handleFilePath(p); } else { setError('浏览器里拿不到文件路径 —— 请点「浏览本机文件」选择'); setBrowsing(true); }
                }
              }}/>
            {filePath
              ? <span style={{color: 'var(--mc-text)'}}>{filePath}</span>
              : '把 zip / mrpack 拖到这里，或点击选择文件'}
          </label>
          <button type="button" className="p-btn" onClick={() => setBrowsing(true)}>浏览本机文件</button>
        </>
      ) : (
        <Input placeholder="包链接" value={url} onChange={e => setUrl(e.target.value)} style={{width: 280}}/>
      )}
      <Button loading={busy}
        disabled={source === 'local' ? !filePath : !url.trim()}
        onClick={() => act(async () => {
          setNotice(null);
          setPreview(source === 'local'
            ? await inspectImport({source: 'local', path: filePath})
            : await inspectImport({source, url: url.trim()}));
        })}>解析</Button>
      {preview && (
        <>
          <span className="sub">{preview.packName || '(未命名)'} · {preview.entryCount} 个条目</span>
          <Button type="primary" loading={busy} disabled={source === 'local' ? !filePath : !url.trim()}
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
      {browsing && (
        <Modal onClose={() => setBrowsing(false)}>
          <div className="p-title">选择整合包文件（.zip / .mrpack）</div>
          <ImportFileBrowser onPick={p => { setBrowsing(false); void handleFilePath(p); }} onClose={() => setBrowsing(false)}/>
        </Modal>
      )}
    </div>
    </>
  );
}

/* 服务端文件选择（2026-10-05 定稿：路径来自本机列表，直接发后端读盘）。
   目录可进入，zip/mrpack 文件可选中；其余文件置灰展示。 */
function ImportFileBrowser({onPick, onClose}: {onPick: (path: string) => void; onClose: () => void}) {
  const [browse, setBrowse] = useState<FsBrowse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const go = (path?: string) => browseDirectories(path).then(setBrowse).catch(e => setError(String(e)));
  useEffect(() => { go(); }, []);
  const ext = (name: string) => { const n = name.toLowerCase(); return n.endsWith('.zip') || n.endsWith('.mrpack'); };
  return (
    <>
      <div className="dm-path">
        <span className="mono grow">{browse?.path ?? '…'}</span>
        <button type="button" className="p-btn" onClick={() => browse?.parent && go(browse.parent)}>上一级</button>
      </div>
      {error && <div className="p-empty">{error}</div>}
      <div className="dm-list">
        {(browse?.directories ?? []).map(d => (
          <div key={d.path} className="p-row click" onClick={() => go(d.path)} title={d.path}>
            <span className="grow">{d.name}</span>
            <span className="sub">进入</span>
          </div>
        ))}
        {(browse?.files ?? []).map(f => (
          <div key={f.path} className={`p-row${ext(f.name) ? ' click' : ' na'}`}
            title={ext(f.name) ? `选择 ${f.name}（${Math.max(1, Math.round(f.size / 1024 / 1024))} MB）` : '不是 .zip / .mrpack'}
            onClick={() => { if (ext(f.name)) onPick(f.path); }}>
            <span className="grow">{f.name}</span>
            <span className="sub">{Math.max(1, Math.round(f.size / 1024 / 1024))} MB</span>
          </div>
        ))}
        {browse && (browse.files ?? []).length === 0 && browse.directories.length === 0 && (
          <div className="p-empty">这个目录是空的。</div>
        )}
      </div>
      <div className="mm-actions">
        <button type="button" className="p-btn" onClick={onClose}>取消</button>
      </div>
    </>
  );
}
