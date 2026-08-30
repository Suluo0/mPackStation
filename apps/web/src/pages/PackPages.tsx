import {useEffect, useState} from 'react';
import {App, Button, Divider, Form, Input, Progress, Select, Space, Switch, Tag} from 'antd';
import {
  ArrowRightOutlined, CheckCircleFilled, CheckOutlined,
  CodeOutlined, DeleteOutlined, FileZipOutlined,
  InfoCircleOutlined, LinkOutlined, MoreOutlined, PlusOutlined,
  ReloadOutlined, SearchOutlined, SettingOutlined, UploadOutlined, WarningFilled,
} from '@ant-design/icons';
import {useNavigate, useParams} from 'react-router-dom';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import './pack-pages.css';
import {addMod, getPack, listConflicts, listLocks, listMods, listModVersions, listPacks, removeMod, resolvePack, searchAllMods, updateMod} from '../features/pack/api';
import type {ModVersion, Pack, Mod, SearchAllItem} from '../features/pack/api';
import {listContent, validateContent, applyContent} from '../features/content/api';
import {getQuest, validateQuest, applyQuest} from '../features/content/quests';
import {listDeliveryChecks, runDeliveryChecks, listVersions, listArtifacts, buildPack} from '../features/release/api';

function PackContext({active = '概览', action}: {active?: string; action?: React.ReactNode}) {
  const {id} = useParams();
  const navigate = useNavigate();
  const [pack, setPack] = useState<Pack | null>(null);
  useEffect(() => { if (id) void getPack(id).then(setPack).catch(() => setPack(null)); }, [id]);
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

const mods = [
  {name: 'Create', author: 'simibubi', version: '0.5.1-f', downloads: '18.4M', status: '已安装', color: 'orange', desc: '让机械动力与自动化成为整合包的核心。'},
  {name: 'JEI', author: 'mezz', version: '15.3.0.4', downloads: '52.1M', status: '已安装', color: 'green', desc: '在游戏内查看配方、用途与合成路径。'},
  {name: 'Sophisticated Backpacks', author: 'P3pp3rF1y', version: '3.20.5.1', downloads: '9.2M', status: '待确认', color: 'gold', desc: '可升级的背包与仓储，适合长线探索。'},
  {name: 'Applied Energistics 2', author: 'unpredictable', version: '15.2.14', downloads: '23.6M', status: '可添加', color: 'blue', desc: '构建网络化存储与自动化生产线。'},
];

export function PacksPage() {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const [packs, setPacks] = useState<Pack[]>([]); const [error, setError] = useState('');
  useEffect(() => { void listPacks().then(setPacks).catch(e => setError(e instanceof Error ? e.message : String(e))); }, []);
  const visible = packs.filter(p => p.name.includes(query));
  return <div className="workspace-page">
    <div className="page-heading"><div><span className="eyebrow">WORKSPACE / PACKS</span><h1>整合包</h1><p>每个包都是一条独立的设计路径，从选模组一直走到交付。</p></div><Space><Button icon={<UploadOutlined/>}>导入整合包</Button><WorkbenchButton tone="primary" icon={<PlusOutlined/>}>新建整合包</WorkbenchButton></Space></div>
    <WorkbenchCard className="pack-overview-strip"><div><span className="strip-label">当前工作集</span><strong>{packs.length} 个整合包</strong></div><div><span className="strip-label">最近解决</span><strong className="text-success">—</strong></div><div><span className="strip-label">需要处理</span><strong className="text-warning">—</strong></div><Input prefix={<SearchOutlined/>} placeholder="筛选整合包" value={query} onChange={e => setQuery(e.target.value)} allowClear /></WorkbenchCard>
    <WorkbenchCard className="pack-table-card"><WorkbenchSectionHeader title="全部整合包" action={<span className="db-muted">按最后编辑排序</span>}/>{error && <div className="empty-inline">加载失败：{error}</div>}<div className="pack-table pack-table-head"><span>包名称</span><span>环境</span><span>模组</span><span>状态</span><span>最后编辑</span><span /></div>{visible.map(p => <button className="pack-table pack-table-row" key={p.id} onClick={() => navigate(`/packs/${p.id}`)}><span className="pack-table-name"><span className={`pack-mini-cover pack-mini-${p.id}`}>{p.name.slice(0, 1)}</span><strong>{p.name}</strong><Tag>{p.packVersion}</Tag></span><span><Tag>MC {p.mcVersion}</Tag><Tag>{p.loader}</Tag></span><span className="tabular">—</span><span className="text-success tabular">{p.status}</span><span className="db-muted">{p.updatedAt ?? p.createdAt ?? '—'}</span><span><MoreOutlined /></span></button>)}{!error && visible.length === 0 && <div className="empty-inline">没有匹配的整合包。试试其他名称。</div>}</WorkbenchCard>
  </div>;
}

export function PackWorkbenchPage() {
  const {id} = useParams(); const navigate = useNavigate(); const {message} = App.useApp(); const [pack,setPack]=useState<Pack|null>(null); const [items,setItems]=useState<Mod[]>([]); const [error,setError]=useState('');
  useEffect(()=>{ if (!id) return; void Promise.all([getPack(id),listMods(id)]).then(([p,m])=>{setPack(p);setItems(m)}).catch(e=>setError(e instanceof Error?e.message:String(e))); },[id]);
  const rows = items.slice(0,3).map(m=>({name:m.displayName,author:m.source,version:m.versionId||'—',downloads:'',status:m.status,color:'blue',desc:m.fileName}));
  return <div className="workspace-page"><PackContext action={<WorkbenchButton tone="primary" onClick={() => navigate(`/packs/${id}/publish`)} icon={<FileZipOutlined/>}>开始打包</WorkbenchButton>}/><div className="page-heading compact"><div><span className="eyebrow">PACK WORKBENCH</span><h1>{pack?.name ?? '加载中…'}</h1><p>选择模组、锁定依赖，处理会阻塞交付的冲突。</p></div><Button icon={<SettingOutlined/>} onClick={() => message.info('包设置将在此处打开')}>包设置</Button></div>{error&&<div className="empty-inline">加载失败：{error}</div>}<div className="workbench-grid"><main className="workbench-main"><WorkbenchCard className="mod-search-card"><div className="result-note"><span><strong>{items.length}</strong> 个已选择模组</span><Button type="link" onClick={() => navigate(`/packs/${id}/mods`)}>查看完整搜索 <ArrowRightOutlined/></Button></div>{rows.map(m=><ModRow key={m.name} mod={m} onAction={()=>message.success(`${m.name} 已加入整合包`)}/>)}{!error&&items.length===0&&<div className="empty-inline">当前还没有模组。</div>}<Button block type="dashed" className="list-more" onClick={()=>navigate(`/packs/${id}/mods`)}>搜索并添加更多模组</Button></WorkbenchCard></main><PackHealthRail id={id}/></div></div>;
}

function ModRow({mod, onAction}: {mod: typeof mods[number]; onAction: () => void}) { return <div className="mod-row"><span className="mod-symbol"><CodeOutlined/></span><div className="mod-row-main"><strong>{mod.name}</strong><span>{mod.author} · {mod.version} · {mod.downloads} 下载</span><small>{mod.desc}</small></div><Tag color={mod.color}>{mod.status}</Tag><Button size="small" onClick={onAction}>{mod.status === '已安装' ? '查看' : '添加'}</Button></div>; }

function PackHealthRail({id}: {id?: string}) { const navigate = useNavigate(); return <aside className="pack-health-rail"><div className="health-kicker">PACK HEALTH</div><h2>包健康</h2><div className="health-score"><strong>86</strong><span>/ 100</span><Tag color="gold">需要关注</Tag></div><div className="health-list"><div><CheckCircleFilled className="text-success"/><span>依赖已锁定</span><b>142</b></div><div><WarningFilled className="text-danger"/><span>待解决冲突</span><b className="text-danger">3</b></div><div><InfoCircleOutlined className="text-info"/><span>可更新模组</span><b>5</b></div><div><CheckOutlined className="text-success"/><span>内容编辑</span><b>26</b></div></div><Divider/><Button block onClick={() => navigate(`/packs/${id}/dependencies`)}>处理冲突 <ArrowRightOutlined/></Button></aside>; }

function fmtDownloads(n?: number) { if (!n) return '0'; if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M'; if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K'; return String(n); }

export function PackModsPage() {
  const {id} = useParams();
  const {message} = App.useApp();
  const [pack, setPack] = useState<Pack | null>(null);
  const [installed, setInstalled] = useState<Mod[]>([]);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchAllItem[]>([]);
  const [searchErrors, setSearchErrors] = useState<Record<string, string>>({});
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState(false);
  const [versions, setVersions] = useState<Record<string, ModVersion[]>>({});
  const [choice, setChoice] = useState<Record<string, string>>({});
  const [error, setError] = useState('');

  const refresh = () => { if (id) void listMods(id).then(setInstalled).catch(e => setError(e instanceof Error ? e.message : String(e))); };
  useEffect(() => { if (!id) return; void getPack(id).then(setPack).catch(() => undefined); refresh(); }, [id]);

  const compatible = (v: ModVersion) => {
    if (!pack) return true;
    const mcOk = !v.gameVersions?.length || v.gameVersions.includes(pack.mcVersion);
    const ldOk = !v.loaders?.length || v.loaders.some(l => l.toLowerCase() === pack.loader.toLowerCase());
    return mcOk && ldOk;
  };

  const keyOf = (p: SearchAllItem) => `${p.provider}:${p.id}`;

  const runSearch = () => {
    if (!id || !query.trim()) return;
    setSearching(true); setError('');
    searchAllMods(id, {query: query.trim(), limit: 20, mcVersion: pack?.mcVersion, loader: pack?.loader})
      .then(v => { setResults(v.items); setSearchErrors(v.errors ?? {}); setSearched(true); setVersions({}); setChoice({}); })
      .catch(e => setError(e instanceof Error ? e.message : String(e)))
      .finally(() => setSearching(false));
  };

  const loadVersions = (p: SearchAllItem) => {
    if (!id || versions[keyOf(p)]) return;
    listModVersions(id, p.provider, p.id).then(vs => {
      setVersions(prev => ({...prev, [keyOf(p)]: vs}));
      const first = vs.find(compatible) ?? vs[0];
      if (first) setChoice(prev => ({...prev, [keyOf(p)]: first.id}));
    }).catch(e => message.error(e instanceof Error ? e.message : String(e)));
  };

  const add = (p: SearchAllItem) => {
    if (!id) return;
    const versionId = choice[keyOf(p)];
    if (!versionId) { message.error('请先选择版本'); return; }
    addMod(id, {provider: p.provider, projectId: p.id, versionId, required: true})
      .then(() => { message.success('已添加到整合包'); refresh(); })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)));
  };

  const toggleInstalled = (m: Mod) => {
    if (!id) return;
    const op = m.status === 'disabled'
      ? updateMod(id, m.id, {status: 'enabled'})
      : removeMod(id, m.id);
    op.then(() => { message.success(m.status === 'disabled' ? '已启用' : '已移除'); refresh(); })
      .catch(e => message.error(e instanceof Error ? e.message : String(e)));
  };

  const errorText: Record<string, string> = {
    not_configured: '未配置(设置 CURSEFORGE_API_KEY 后可用)',
    rate_limited: '请求过快,稍后再试',
    unauthorized: 'API key 无效',
    unavailable: '平台暂时不可用',
    not_found: '资源不存在',
  };

  return <div className="workspace-page">
    <PackContext active="模组"/>
    <div className="page-heading compact"><div><span className="eyebrow">MOD CATALOG</span><h1>模组</h1><p>按名称同时搜索 Modrinth 和 CurseForge,版本按当前包 MC {pack?.mcVersion ?? '…'} · {pack?.loader ?? '…'} 过滤。</p></div></div>
    <WorkbenchCard className="catalog-card">
      <div className="catalog-toolbar">
        <Input size="large" allowClear autoFocus prefix={<SearchOutlined/>} placeholder="输入模组名称(模糊搜索,双平台并行)" value={query} onChange={e => setQuery(e.target.value)} onPressEnter={runSearch}/>
        <Button icon={<ReloadOutlined/>} loading={searching} onClick={runSearch}>搜索</Button>
      </div>
      {error && <div className="empty-inline">加载失败:{error}</div>}
      {Object.entries(searchErrors).map(([p, code]) =>
        <div className="empty-inline" key={p}>{p === 'curseforge' ? 'CurseForge' : 'Modrinth'}:{errorText[code] ?? code}</div>)}
      {searched && <div className="result-note"><span><strong>{results.length}</strong> 个搜索结果(按下载量排序)</span></div>}
      {results.map(p => {
        const k = keyOf(p);
        const vs = versions[k];
        return <div className="mod-row" key={k}>
          <span className="mod-symbol"><CodeOutlined/></span>
          <div className="mod-row-main"><strong>{p.name}</strong><span>{fmtDownloads(p.downloads)} 下载</span><small>{p.summary}</small></div>
          <Tag color={p.provider === 'modrinth' ? 'green' : 'orange'}>{p.provider === 'modrinth' ? 'Modrinth' : 'CurseForge'}</Tag>
          <Select size="small" style={{minWidth: 240}} placeholder="选择版本" value={choice[k]}
            onFocus={() => loadVersions(p)}
            onChange={v => setChoice(prev => ({...prev, [k]: v}))}
            options={(vs ?? []).map(v => ({value: v.id, label: `${v.versionNumber ?? v.name ?? v.id}${compatible(v) ? '' : '(可能不兼容)'}`}))}
            notFoundContent={vs ? '无匹配版本' : '点击加载版本'}/>
          <Button size="small" type="primary" disabled={!choice[k]} onClick={() => add(p)}>添加</Button>
        </div>;
      })}
      {searched && !results.length && !error && <div className="empty-inline">没有搜索结果。</div>}
    </WorkbenchCard>
    <WorkbenchCard className="catalog-card">
      <WorkbenchSectionHeader title={`已安装(${installed.length})`}/>
      {installed.map(m => <div className="mod-row" key={m.id}>
        <span className="mod-symbol"><CodeOutlined/></span>
        <div className="mod-row-main"><strong>{m.displayName}</strong><span>{m.source} · {m.versionId || '—'}</span><small>{m.fileName}</small></div>
        <Tag color={m.status === 'disabled' ? 'gold' : 'green'}>{m.status}</Tag>
        <Button size="small" danger={m.status !== 'disabled'} onClick={() => toggleInstalled(m)}>{m.status === 'disabled' ? '启用' : '移除'}</Button>
      </div>)}
      {!installed.length && <div className="empty-inline">当前还没有模组。搜索并添加第一个。</div>}
    </WorkbenchCard>
  </div>;
}

export function DependenciesPage() { const {id} = useParams(); const {message} = App.useApp(); const [items,setItems]=useState<any[]>([]); const [locks,setLocks]=useState<any[]>([]); const [error,setError]=useState(''); useEffect(()=>{if(!id)return; void Promise.all([listConflicts(id),listLocks(id)]).then(([c,l])=>{setItems(c);setLocks(l)}).catch(e=>setError(e instanceof Error?e.message:String(e)));},[id]); const resolve=()=>{if(!id)return; void resolvePack(id).then(()=>message.success('依赖已重新解析')).catch(e=>setError(e instanceof Error?e.message:String(e)));}; return <div className="workspace-page"><PackContext active="依赖与冲突"/><div className="page-heading compact"><div><span className="eyebrow">DEPENDENCIES / RESOLUTION</span><h1>依赖与冲突</h1><p>把版本问题变成明确的选择，解决后再进入发布检查。</p></div><WorkbenchButton tone="primary" icon={<CheckOutlined/>} onClick={resolve}>重新解析依赖</WorkbenchButton></div>{error&&<div className="empty-inline">加载失败：{error}</div>}<div className="dependency-grid"><main><div className="resolution-summary"><div><span>锁定快照</span><strong className="text-success">{locks.length}</strong></div><div><span>待处理冲突</span><strong className="text-danger">{items.filter(c=>c.status!=='resolved').length}</strong></div></div><WorkbenchCard className="conflict-card"><WorkbenchSectionHeader title="待处理项" action={<Button type="text" icon={<ReloadOutlined/>} onClick={resolve}>重新检查</Button>}/>{items.map(c=><div className="conflict-row" key={c.id}><span className={`conflict-mark ${c.severity}`}><WarningFilled/></span><div><strong>{c.summary}</strong><Tag color={c.severity==='high'?'red':'gold'}>{c.kind}</Tag><p>{c.status}</p></div></div>)}{!error&&!items.length&&<div className="resolved-state"><CheckCircleFilled/><strong>没有冲突</strong></div>}</WorkbenchCard></main></div></div>; }




export function ContentEditorPage() { const {id=''}=useParams(); const [state,setState]=useState<any>(null); const [error,setError]=useState(''); useEffect(()=>{void listContent(id).then(v=>setState(v[0]??null)).catch(e=>setError(String(e)));},[id]); if(error)return <div className="workspace-page"><p>{error}</p></div>; if(!state)return <div className="workspace-page"><p>正在加载内容…</p></div>; return <div className="workspace-page editor-page"><PackContext active="内容编辑"/><div className="page-heading compact"><h1>{state.title}</h1><p>{state.kind} · {state.slug}</p></div><WorkbenchCard><p>内容文档已连接后端，当前修订：{state.activeRevisionId||'草稿'}</p><Space><Button onClick={()=>void validateContent(id,state.id)}>校验</Button><Button type="primary" onClick={()=>void applyContent(id,state.id)}>应用</Button></Space></WorkbenchCard></div>; }
export function QuestEditorPage() { const {id=''}=useParams(); const [book,setBook]=useState<any>(null); const [error,setError]=useState(''); useEffect(()=>{void getQuest(id).then(setBook).catch(e=>setError(String(e)));},[id]); if(error)return <div className="workspace-page"><p>{error}</p></div>; if(!book)return <div className="workspace-page"><p>正在加载任务书…</p></div>; return <div className="workspace-page editor-page"><PackContext active="任务书"/><div className="page-heading compact"><h1>任务书</h1></div><WorkbenchCard><pre style={{whiteSpace:'pre-wrap'}}>{JSON.stringify(book,null,2)}</pre><Space><Button onClick={()=>void validateQuest(id)}>校验</Button><Button type="primary" onClick={()=>void applyQuest(id)}>应用</Button></Space></WorkbenchCard></div>; }
export function PublishPage() { const {id=''}=useParams(); const [checks,setChecks]=useState<any[]>([]); const [versions,setVersions]=useState<any[]>([]); const [artifacts,setArtifacts]=useState<any[]>([]); const [error,setError]=useState(''); useEffect(()=>{void Promise.all([listDeliveryChecks(id).then(v=>setChecks(v.items)),listVersions(id).then(setVersions),listArtifacts(id).then(setArtifacts)]).catch(e=>setError(String(e)));},[id]); if(error)return <div className="workspace-page"><p>{error}</p></div>; return <div className="workspace-page"><PackContext active="打包与发布"/><div className="page-heading compact"><h1>打包与发布</h1><p>交付检查与产物来自后端。</p></div><WorkbenchCard><h3>交付检查</h3>{checks.map(c=><p key={c.kind}>{c.kind}: {c.status} · {c.detail}</p>)}{!checks.length&&<p>暂无交付检查</p>}<Button onClick={()=>void runDeliveryChecks(id,{packVersionId:versions[0]?.id||'',checks})}>重新检查</Button></WorkbenchCard><WorkbenchCard><h3>版本</h3><p>{versions.length?versions.map(v=>v.version).join(', '):'暂无版本'}</p><h3>产物</h3>{artifacts.map(a=><p key={a.id}>{a.fileName} · {a.sha256}</p>)}<Button disabled={!versions[0]} onClick={()=>void buildPack(id,{packVersionId:versions[0]?.id,files:[]})}>开始构建</Button></WorkbenchCard></div>; }

export function SettingsPage() { const {message} = App.useApp(); const [form] = Form.useForm(); return <div className="workspace-page settings-page"><div className="page-heading"><div><span className="eyebrow">WORKSPACE / PREFERENCES</span><h1>设置</h1><p>连接平台、管理存储，并定义新整合包的默认配置。</p></div><Button icon={<SettingOutlined/>}>恢复默认</Button></div><div className="settings-layout"><aside className="settings-rail"><button className="selected">平台连接</button><button>存储与缓存</button><button>默认包配置</button><button>界面</button></aside><main className="settings-main"><section className="settings-section"><WorkbenchSectionHeader title="平台连接"/><ProviderCard name="CurseForge" status="已连接" color="orange" onTest={() => message.success('CurseForge 连接正常')}/><ProviderCard name="Modrinth" status="已连接" color="blue" onTest={() => message.success('Modrinth 连接正常')}/></section><section className="settings-section"><WorkbenchSectionHeader title="存储与缓存" action={<Button type="link" icon={<DeleteOutlined/>}>清理缓存</Button>}/><div className="storage-line"><div><span>模组缓存</span><strong>1.2 GB / 10 GB</strong></div><Progress percent={12} showInfo={false}/><small>D:\\mPackStation\\cache · 剩余空间 86.4 GB</small></div></section><section className="settings-section"><WorkbenchSectionHeader title="默认包配置"/><Form form={form} layout="vertical" initialValues={{version: '1.20.1', loader: 'NeoForge', autoResolve: true}} onFinish={() => message.success('默认配置已保存')}><div className="form-grid"><Form.Item label="MC 版本" name="version"><Select options={['1.20.1', '1.21.1'].map(v => ({label: v, value: v}))}/></Form.Item><Form.Item label="加载器" name="loader"><Select options={['NeoForge', 'Forge', 'Fabric', 'Quilt'].map(v => ({label: v, value: v}))}/></Form.Item></div><Form.Item label="创建后自动处理" name="autoResolve" valuePropName="checked"><Switch checkedChildren="自动" unCheckedChildren="手动"/></Form.Item><Button type="primary" htmlType="submit" className="mc-btn-cta">保存默认配置</Button></Form></section></main></div></div>; }
function ProviderCard({name, status, color, onTest}: {name: string; status: string; color: string; onTest: () => void}) { return <div className="provider-card"><span className={`provider-mark provider-${color}`}><LinkOutlined/></span><div><strong>{name}</strong><span>API Key 已配置 · 上次检查 10 分钟前</span></div><Tag color="green">{status}</Tag><Button size="small" onClick={onTest}>测试连接</Button></div>; }
