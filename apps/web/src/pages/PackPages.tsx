import {useEffect, useState} from 'react';
import {App, Button, Divider, Input, Select, Tag} from 'antd';
import {
  ArrowRightOutlined, CheckCircleFilled, CheckOutlined,
  CodeOutlined, FileZipOutlined,
  InfoCircleOutlined, LinkOutlined, MoreOutlined, PlusOutlined,
  ReloadOutlined, SearchOutlined, SettingOutlined, UploadOutlined, WarningFilled,
} from '@ant-design/icons';
import {useNavigate, useParams} from 'react-router-dom';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import './pack-pages.css';
import {listMods, type Mod} from '../api/mods';
import {listDeliveryChecks, runDeliveryChecks, listVersions, listArtifacts, buildPack} from '../api/releases';
import {fetchHealth, fetchStatus, saveCurseForgeKey, clearCurseForgeKey, type SystemHealth, type SystemStatus} from '../api/system';
import {usePack} from '../hooks/usePack';
import {usePacks} from '../hooks/usePacks';
import {useModSearch} from '../hooks/useModSearch';
import {useDependencies} from '../hooks/useDependencies';
import {useContentEditor, useQuestBook} from '../hooks/useEditors';

function PackContext({active = '概览', action}: {active?: string; action?: React.ReactNode}) {
  const {id} = useParams();
  const navigate = useNavigate();
  const {pack} = usePack(id);
  const name = pack?.name ?? '整合包';
  return <header className="pack-context">
    <button className="pack-context-back" onClick={() => navigate('/packs')} aria-label="返回整合包列表">整合包</button>
    <span className="pack-context-sep">/</span>
    <div className="pack-context-cover">{name.slice(0, 1)}</div>
    <div className="pack-context-title"><strong>{name}</strong><span>{pack ? `MC ${pack.mcVersion} · ${pack.loader} · v${pack.packVersion}` : '加载中…'}</span></div>
    <Tag color="green">已保存</Tag>
    <div className="pack-context-tabs">{active}</div>
    <div className="pack-context-action">{action}</div>
  </header>;
}

export function PacksPage() {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const {packs, error} = usePacks();
  const visible = packs.filter(p => p.name.includes(query));
  return <div className="workspace-page">
    <div className="page-heading"><div><span className="eyebrow">WORKSPACE / PACKS</span><h1>整合包</h1><p>每个包都是一条独立的设计路径,从选模组一直走到交付。</p></div><Space2/></div>
    <WorkbenchCard className="pack-overview-strip"><div><span className="strip-label">当前工作集</span><strong>{packs.length} 个整合包</strong></div><div><span className="strip-label">最近解决</span><strong className="text-success">—</strong></div><div><span className="strip-label">需要处理</span><strong className="text-warning">—</strong></div><Input prefix={<SearchOutlined/>} placeholder="筛选整合包" value={query} onChange={e => setQuery(e.target.value)} allowClear /></WorkbenchCard>
    <WorkbenchCard className="pack-table-card"><WorkbenchSectionHeader title="全部整合包" action={<span className="db-muted">按最后编辑排序</span>}/>{error && <div className="empty-inline">加载失败:{error}</div>}<div className="pack-table pack-table-head"><span>包名称</span><span>环境</span><span>模组</span><span>状态</span><span>最后编辑</span><span /></div>{visible.map(p => <button className="pack-table pack-table-row" key={p.id} onClick={() => navigate(`/packs/${p.id}`)}><span className="pack-table-name"><span className={`pack-mini-cover pack-mini-${p.id}`}>{p.name.slice(0, 1)}</span><strong>{p.name}</strong><Tag>{p.packVersion}</Tag></span><span><Tag>MC {p.mcVersion}</Tag><Tag>{p.loader}</Tag></span><span className="tabular">—</span><span className="text-success tabular">{p.status}</span><span className="db-muted">{p.updatedAt ?? p.createdAt ?? '—'}</span><span><MoreOutlined /></span></button>)}{!error && visible.length === 0 && <div className="empty-inline">没有匹配的整合包。试试其他名称。</div>}</WorkbenchCard>
  </div>;
}

/* 页头右侧动作按钮组(导入/新建),保持原布局。 */
function Space2() {
  return <span style={{display: 'inline-flex', gap: 8}}><Button icon={<UploadOutlined/>}>导入整合包</Button><WorkbenchButton tone="primary" icon={<PlusOutlined/>}>新建整合包</WorkbenchButton></span>;
}

export function PackWorkbenchPage() {
  const {id} = useParams(); const navigate = useNavigate(); const {message} = App.useApp();
  const {pack} = usePack(id);
  const [items, setItems] = useState<Mod[]>([]); const [error, setError] = useState('');
  useEffect(() => { if (id) void listMods(id).then(setItems).catch(e => setError(e instanceof Error ? e.message : String(e))); }, [id]);
  const rows = items.slice(0, 3).map(m => ({name: m.displayName, author: m.source, version: m.versionId || '—', downloads: '', status: m.status, color: 'blue', desc: m.fileName}));
  return <div className="workspace-page"><PackContext action={<WorkbenchButton tone="primary" onClick={() => navigate(`/packs/${id}/publish`)} icon={<FileZipOutlined/>}>开始打包</WorkbenchButton>}/><div className="page-heading compact"><div><span className="eyebrow">PACK WORKBENCH</span><h1>{pack?.name ?? '加载中…'}</h1><p>选择模组、锁定依赖,处理会阻塞交付的冲突。</p></div><Button icon={<SettingOutlined/>} onClick={() => message.info('包设置将在此处打开')}>包设置</Button></div>{error && <div className="empty-inline">加载失败:{error}</div>}<div className="workbench-grid"><main className="workbench-main"><WorkbenchCard className="mod-search-card"><div className="result-note"><span><strong>{items.length}</strong> 个已选择模组</span><Button type="link" onClick={() => navigate(`/packs/${id}/mods`)}>查看完整搜索 <ArrowRightOutlined/></Button></div>{rows.map(m => <ModRow key={m.name} mod={m} onAction={() => message.success(`${m.name} 已加入整合包`)}/>)}{!error && items.length === 0 && <div className="empty-inline">当前还没有模组。</div>}<Button block type="dashed" className="list-more" onClick={() => navigate(`/packs/${id}/mods`)}>搜索并添加更多模组</Button></WorkbenchCard></main><PackHealthRail id={id}/></div></div>;
}

type ModRowData = {name: string; author: string; version: string; downloads: string; status: string; color: string; desc: string};
function ModRow({mod, onAction}: {mod: ModRowData; onAction: () => void}) { return <div className="mod-row"><span className="mod-symbol"><CodeOutlined/></span><div className="mod-row-main"><strong>{mod.name}</strong><span>{mod.author} · {mod.version} · {mod.downloads} 下载</span><small>{mod.desc}</small></div><Tag color={mod.color}>{mod.status}</Tag><Button size="small" onClick={onAction}>{mod.status === '已安装' ? '查看' : '添加'}</Button></div>; }

function PackHealthRail({id}: {id?: string}) { const navigate = useNavigate(); return <aside className="pack-health-rail"><div className="health-kicker">PACK HEALTH</div><h2>包健康</h2><div className="health-score"><strong>86</strong><span>/ 100</span><Tag color="gold">需要关注</Tag></div><div className="health-list"><div><CheckCircleFilled className="text-success"/><span>依赖已锁定</span><b>142</b></div><div><WarningFilled className="text-danger"/><span>待解决冲突</span><b className="text-danger">3</b></div><div><InfoCircleOutlined className="text-info"/><span>可更新模组</span><b>5</b></div><div><CheckOutlined className="text-success"/><span>内容编辑</span><b>26</b></div></div><Divider/><Button block onClick={() => navigate(`/packs/${id}/dependencies`)}>处理冲突 <ArrowRightOutlined/></Button></aside>; }

function fmtDownloads(n?: number) { if (!n) return '0'; if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M'; if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K'; return String(n); }

const searchErrorText: Record<string, string> = {
  not_configured: '未配置(设置 CURSEFORGE_API_KEY 后可用)',
  rate_limited: '请求过快,稍后再试',
  unauthorized: 'API key 无效',
  unavailable: '平台暂时不可用',
  not_found: '资源不存在',
};

export function PackModsPage() {
  const {id} = useParams();
  const {pack} = usePack(id);
  const s = useModSearch(id, pack);

  return <div className="workspace-page">
    <PackContext active="模组"/>
    <div className="page-heading compact"><div><span className="eyebrow">MOD CATALOG</span><h1>模组</h1><p>按名称同时搜索 Modrinth 和 CurseForge,版本按当前包 MC {pack?.mcVersion ?? '…'} · {pack?.loader ?? '…'} 过滤。</p></div></div>
    <WorkbenchCard className="catalog-card">
      <div className="catalog-toolbar">
        <Input size="large" allowClear autoFocus prefix={<SearchOutlined/>} placeholder="输入模组名称(模糊搜索,双平台并行)" value={s.query} onChange={e => s.setQuery(e.target.value)} onPressEnter={s.runSearch}/>
        <Button icon={<ReloadOutlined/>} loading={s.searching} onClick={s.runSearch}>搜索</Button>
      </div>
      {s.error && <div className="empty-inline">加载失败:{s.error}</div>}
      {Object.entries(s.searchErrors).map(([p, code]) =>
        <div className="empty-inline" key={p}>{p === 'curseforge' ? 'CurseForge' : 'Modrinth'}:{searchErrorText[code] ?? code}</div>)}
      {s.searched && <div className="result-note"><span><strong>{s.results.length}</strong> 个搜索结果(按下载量排序)</span></div>}
      {s.results.map(p => {
        const k = s.keyOf(p);
        const vs = s.versions[k];
        return <div className="mod-row" key={k}>
          <span className="mod-symbol"><CodeOutlined/></span>
          <div className="mod-row-main"><strong>{p.name}</strong><span>{fmtDownloads(p.downloads)} 下载</span><small>{p.summary}</small></div>
          <Tag color={p.provider === 'modrinth' ? 'green' : 'orange'}>{p.provider === 'modrinth' ? 'Modrinth' : 'CurseForge'}</Tag>
          <Select size="small" style={{minWidth: 240}} placeholder="选择版本" value={s.choice[k]}
            onFocus={() => s.loadVersions(p)}
            onChange={v => s.setChoice(prev => ({...prev, [k]: v}))}
            options={(vs ?? []).map(v => ({value: v.id, label: `${v.versionNumber ?? v.name ?? v.id}${s.compatible(v) ? '' : '(可能不兼容)'}`}))}
            notFoundContent={vs ? '无匹配版本' : '点击加载版本'}/>
          <Button size="small" type="primary" disabled={!s.choice[k]} onClick={() => s.add(p)}>添加</Button>
        </div>;
      })}
      {s.searched && !s.results.length && !s.error && <div className="empty-inline">没有搜索结果。</div>}
    </WorkbenchCard>
    <WorkbenchCard className="catalog-card">
      <WorkbenchSectionHeader title={`已安装(${s.installed.length})`}/>
      {s.installed.map(m => <div className="mod-row" key={m.id}>
        <span className="mod-symbol"><CodeOutlined/></span>
        <div className="mod-row-main"><strong>{m.displayName}</strong><span>{m.source} · {m.versionId || '—'}</span><small>{m.fileName}</small></div>
        <Tag color={m.status === 'disabled' ? 'gold' : 'green'}>{m.status}</Tag>
        <Button size="small" danger={m.status !== 'disabled'} onClick={() => s.toggleInstalled(m)}>{m.status === 'disabled' ? '启用' : '移除'}</Button>
      </div>)}
      {!s.installed.length && <div className="empty-inline">当前还没有模组。搜索并添加第一个。</div>}
    </WorkbenchCard>
  </div>;
}

export function DependenciesPage() {
  const {id} = useParams();
  const {conflicts, locks, error, resolve} = useDependencies(id);
  return <div className="workspace-page"><PackContext active="依赖与冲突"/><div className="page-heading compact"><div><span className="eyebrow">DEPENDENCIES / RESOLUTION</span><h1>依赖与冲突</h1><p>把版本问题变成明确的选择,解决后再进入发布检查。</p></div><WorkbenchButton tone="primary" icon={<CheckOutlined/>} onClick={resolve}>重新解析依赖</WorkbenchButton></div>{error && <div className="empty-inline">加载失败:{error}</div>}<div className="dependency-grid"><main><div className="resolution-summary"><div><span>锁定快照</span><strong className="text-success">{locks.length}</strong></div><div><span>待处理冲突</span><strong className="text-danger">{conflicts.filter(c => c.status !== 'resolved').length}</strong></div></div><WorkbenchCard className="conflict-card"><WorkbenchSectionHeader title="待处理项" action={<Button type="text" icon={<ReloadOutlined/>} onClick={resolve}>重新检查</Button>}/>{conflicts.map(c => <div className="conflict-row" key={c.id}><span className={`conflict-mark ${c.severity}`}><WarningFilled/></span><div><strong>{c.summary}</strong><Tag color={c.severity === 'high' ? 'red' : 'gold'}>{c.kind}</Tag><p>{c.status}</p></div></div>)}{!error && !conflicts.length && <div className="resolved-state"><CheckCircleFilled/><strong>没有冲突</strong></div>}</WorkbenchCard></main></div></div>;
}

export function ContentEditorPage() {
  const {id = ''} = useParams();
  const editor = useContentEditor(id);
  if (editor.error) return <div className="workspace-page"><p>{editor.error}</p></div>;
  if (!editor.doc) return <div className="workspace-page"><p>正在加载内容…</p></div>;
  return <div className="workspace-page editor-page"><PackContext active="内容编辑"/><div className="page-heading compact"><h1>{editor.doc.title}</h1><p>{editor.doc.kind} · {editor.doc.slug}</p></div><WorkbenchCard><p>内容文档已连接后端,当前修订:{editor.doc.activeRevisionId || '草稿'}</p><span style={{display: 'inline-flex', gap: 8}}><Button onClick={editor.validate}>校验</Button><Button type="primary" onClick={editor.apply}>应用</Button></span></WorkbenchCard></div>;
}

export function QuestEditorPage() {
  const {id = ''} = useParams();
  const quest = useQuestBook(id);
  if (quest.error) return <div className="workspace-page"><p>{quest.error}</p></div>;
  if (!quest.book) return <div className="workspace-page"><p>正在加载任务书…</p></div>;
  return <div className="workspace-page editor-page"><PackContext active="任务书"/><div className="page-heading compact"><h1>任务书</h1></div><WorkbenchCard><pre style={{whiteSpace: 'pre-wrap'}}>{JSON.stringify(quest.book, null, 2)}</pre><span style={{display: 'inline-flex', gap: 8}}><Button onClick={quest.validate}>校验</Button><Button type="primary" onClick={quest.apply}>应用</Button></span></WorkbenchCard></div>;
}

export function PublishPage() { const {id = ''} = useParams(); const [checks, setChecks] = useState<any[]>([]); const [versions, setVersions] = useState<any[]>([]); const [artifacts, setArtifacts] = useState<any[]>([]); const [error, setError] = useState(''); useEffect(() => { void Promise.all([listDeliveryChecks(id).then(v => setChecks(v.items)), listVersions(id).then(setVersions), listArtifacts(id).then(setArtifacts)]).catch(e => setError(String(e))); }, [id]); if (error) return <div className="workspace-page"><p>{error}</p></div>; return <div className="workspace-page"><PackContext active="打包与发布"/><div className="page-heading compact"><h1>打包与发布</h1><p>交付检查与产物来自后端。</p></div><WorkbenchCard><h3>交付检查</h3>{checks.map(c => <p key={c.kind}>{c.kind}: {c.status} · {c.detail}</p>)}{!checks.length && <p>暂无交付检查</p>}<Button onClick={() => void runDeliveryChecks(id, {packVersionId: versions[0]?.id || '', checks})}>重新检查</Button></WorkbenchCard><WorkbenchCard><h3>版本</h3><p>{versions.length ? versions.map(v => v.version).join(', ') : '暂无版本'}</p><h3>产物</h3>{artifacts.map(a => <p key={a.id}>{a.fileName} · {a.sha256}</p>)}<Button disabled={!versions[0]} onClick={() => void buildPack(id, {packVersionId: versions[0]?.id, files: []})}>开始构建</Button></WorkbenchCard></div>; }

function formatBytes(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  return `${n} B`;
}

const providerStatusText: Record<string, {label: string; color: string}> = {
  ok: {label: '已连接', color: 'green'},
  unavailable: {label: '不可用', color: 'red'},
  unknown: {label: '未探测', color: 'default'},
};

/* 设置页只保留有真实后端的区块(契约 §4.2 / 决策 D-11):平台连接状态与
   存储占用来自 GET /api/system/status;"清理缓存""默认包配置""恢复默认"
   无对应接口,已移除,后续需要时立项补 /api/settings 与缓存清理接口。 */
export function SettingsPage() {
  const {message} = App.useApp();
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [error, setError] = useState('');
  const [cfKey, setCfKey] = useState('');
  const [keyBusy, setKeyBusy] = useState(false);
  const refresh = () => {
    fetchStatus().then(setStatus).catch(e => setError(e instanceof Error ? e.message : String(e)));
    fetchHealth().then(setHealth).catch(() => {});
  };
  useEffect(refresh, []);
  const onSaveKey = () => {
    if (!cfKey.trim()) { message.error('请先粘贴 CurseForge API Key'); return; }
    setKeyBusy(true);
    saveCurseForgeKey(cfKey.trim())
      .then(() => { message.success('Key 已验证并保存,立即生效'); setCfKey(''); refresh(); })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)))
      .finally(() => setKeyBusy(false));
  };
  const onClearKey = () => {
    setKeyBusy(true);
    clearCurseForgeKey()
      .then(() => { message.success('已清除保存的 Key'); refresh(); })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)))
      .finally(() => setKeyBusy(false));
  };
  return <div className="workspace-page settings-page">
    <div className="page-heading"><div><span className="eyebrow">WORKSPACE / PREFERENCES</span><h1>设置</h1><p>平台连接状态与本地存储占用。</p></div></div>
    <div className="settings-layout">
      <aside className="settings-rail"><button className="selected">平台连接</button><button>存储与缓存</button></aside>
      <main className="settings-main">
        {error && <p>系统状态加载失败:{error}</p>}
        <section className="settings-section">
          <WorkbenchSectionHeader title="平台连接"/>
          <ProviderCard name="CurseForge" status={status?.curseforgeStatus ?? 'unknown'} reachable={status?.curseforgeReachable ?? false}/>
          <div className="cf-key-editor" style={{display: 'flex', gap: 8, alignItems: 'center', margin: '8px 0 16px'}}>
            <Input.Password
              placeholder={health?.curseforgeKeyConfigured ? '已配置,粘贴新 Key 可覆盖' : '粘贴 CurseForge API Key(console.curseforge.com 申请)'}
              value={cfKey} onChange={e => setCfKey(e.target.value)} style={{maxWidth: 420}}/>
            <Button type="primary" loading={keyBusy} onClick={onSaveKey}>验证并保存</Button>
            {health?.curseforgeKeyConfigured && <Button danger loading={keyBusy} onClick={onClearKey}>清除</Button>}
          </div>
          <ProviderCard name="Modrinth" status={status?.modrinthStatus ?? 'unknown'} reachable={status?.modrinthReachable ?? false}/>
        </section>
        <section className="settings-section">
          <WorkbenchSectionHeader title="存储与缓存"/>
          <div className="storage-line">
            <div><span>模组缓存</span><strong>{status ? formatBytes(status.cacheSizeBytes) : '加载中…'}</strong></div>
            <small>剩余空间 {status ? formatBytes(status.storageFreeBytes) : '加载中…'}</small>
          </div>
        </section>
      </main>
    </div>
  </div>;
}

function ProviderCard({name, status, reachable}: {name: string; status: 'unknown' | 'ok' | 'unavailable'; reachable: boolean}) {
  const s = providerStatusText[status] ?? providerStatusText.unknown;
  return <div className="provider-card"><span className="provider-mark"><LinkOutlined/></span><div><strong>{name}</strong><span>{reachable ? 'API 可达' : 'API 不可达或尚未探测'}</span></div><Tag color={s.color}>{s.label}</Tag></div>;
}
