import {useEffect, useMemo, useState} from 'react';
import {App, Button, Divider, Input, Select, Tag} from 'antd';
import {
  ArrowRightOutlined, BookOutlined, CheckCircleFilled, CheckOutlined,
  CodeOutlined, EditOutlined, ExperimentOutlined, FileZipOutlined, GoldOutlined,
  InfoCircleOutlined, LinkOutlined, PlusOutlined,
  ReloadOutlined, SearchOutlined, SettingOutlined, UploadOutlined, WarningFilled,
} from '@ant-design/icons';
import {useNavigate, useParams} from 'react-router-dom';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import './pack-pages.css';
import {listMods, packHealth, listConflicts, listLocks, type Mod, type PackHealth} from '../api/mods';
import {listDeliveryChecks, runDeliveryChecks, listVersions, listArtifacts, buildPack, createVersion, type Artifact} from '../api/releases';
import {fetchHealth, fetchStatus, saveCurseForgeKey, clearCurseForgeKey, type SystemHealth, type SystemStatus} from '../api/system';
import {registerExportDir} from '../api/fs';
import {usePack} from '../hooks/usePack';
import {usePacks} from '../hooks/usePacks';
import {useModSearch} from '../hooks/useModSearch';
import {useDependencies} from '../hooks/useDependencies';
import {useContentEditor, useQuestBook} from '../hooks/useEditors';
import {usePackCatalog} from '../features/pack/PackCatalogContext';
import {useFocus} from '../features/focus/FocusContext';
import {QuestBookEditor} from '../features/quest/QuestBookEditor';
import {CreatePackModal, ImportPackModal} from '../features/dashboard/PackModals';
import {DirectoryPicker} from '../features/common/DirectoryPicker';
import {formatBytes, relativeTime} from '../features/dashboard/signals';

/* 包状态/封面色板的展示映射。 */
const statusLabel = (s: string) => ({active: '正常', archived: '已归档', removed: '已删除'}[s] ?? s);
const coverTone = (id: string) => (id.split('').reduce((a, c) => a + c.charCodeAt(0), 0) % 2 ? 'adventure' : 'demo');

export function PacksPage() {
  const navigate = useNavigate();
  const {message} = App.useApp();
  const [query, setQuery] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const {packs, error, reload} = usePacks();
  const visible = packs.filter(p => p.name.includes(query));
  return <div className="workspace-page">
    <div className="page-heading">
      <div>
        <span className="eyebrow">WORKSPACE / PACKS</span>
        <h1>整合包</h1>
        <p>每个包都是一条独立的设计路径,从选模组一直走到交付。</p>
      </div>
      <span style={{display: 'inline-flex', gap: 8}}>
        <Button icon={<UploadOutlined/>} onClick={() => setImportOpen(true)}>导入整合包</Button>
        <WorkbenchButton tone="primary" icon={<PlusOutlined/>} onClick={() => setCreateOpen(true)}>新建整合包</WorkbenchButton>
      </span>
    </div>
    <WorkbenchCard className="pack-overview-strip">
      <div><span className="strip-label">当前工作集</span><strong className="tabular">{packs.length}</strong></div>
      <div><span className="strip-label">筛选结果</span><strong className="tabular">{visible.length}</strong></div>
      <div><span className="strip-label">状态</span><strong>{error ? '加载失败' : '已同步'}</strong></div>
      <Input prefix={<SearchOutlined/>} placeholder="筛选整合包" value={query} onChange={e => setQuery(e.target.value)} allowClear />
    </WorkbenchCard>
    <WorkbenchCard className="pack-table-card">
      <WorkbenchSectionHeader title="全部整合包" action={<span className="db-muted">按最后编辑排序</span>}/>
      {error && <div className="empty-inline">加载失败:{error}</div>}
      <div className="pack-table pack-table-head"><span>包名称</span><span>环境</span><span>版本</span><span>状态</span><span>最后编辑</span><span /></div>
      {visible.map(p => (
        <button className="pack-table pack-table-row" key={p.id} onClick={() => navigate(`/packs/${p.id}`)}>
          <span className="pack-table-name">
            {/* pack-mini-<包id> 这种类名永远命不中样式(库里只有 demo/adventure 两种渐变),
                封面因此一直是无底色方块;改成按 id 稳定地在两种渐变里取一种。 */}
            <span className={`pack-mini-cover pack-mini-${coverTone(p.id)}`}>{p.name.slice(0, 1)}</span>
            <strong>{p.name}</strong>
          </span>
          <span><Tag>MC {p.mcVersion}</Tag><Tag>{p.loader}</Tag></span>
          <span className="tabular">{p.packVersion}</span>
          <span className={p.status === 'active' ? 'text-success tabular' : 'db-muted tabular'}>{statusLabel(p.status)}</span>
          <span className="db-muted">{relativeTime(p.updatedAt || p.createdAt)}</span>
          <span />
        </button>
      ))}
      {!error && visible.length === 0 && (
        <div className="empty-inline">没有匹配的整合包。点击右上角「新建整合包」创建第一个。</div>
      )}
    </WorkbenchCard>
    <CreatePackModal
      open={createOpen}
      existing={packs}
      onClose={() => setCreateOpen(false)}
      onCreated={id => { setCreateOpen(false); reload(); message.success('整合包已创建'); navigate(`/packs/${id}`); }}
    />
    <ImportPackModal
      open={importOpen}
      onClose={() => setImportOpen(false)}
      onImported={id => { setImportOpen(false); reload(); if (id) navigate(`/packs/${id}`); }}
    />
  </div>;
}

export function PackWorkbenchPage() {
  const {id} = useParams();
  const navigate = useNavigate();
  const {pack} = usePack(id);
  const {items: catalogItems, phase} = usePackCatalog();
  const {focusMod} = useFocus();
  const [items, setItems] = useState<Mod[]>([]);
  const [error, setError] = useState('');
  const reloadMods = () => {
    if (!id) return;
    void listMods(id).then(setItems).catch(e => setError(e instanceof Error ? e.message : String(e)));
  };
  useEffect(reloadMods, [id]);
  return <div className="workspace-page">
    <div className="page-heading compact">
      <div>
        <span className="eyebrow">PACK WORKBENCH</span>
        <h1>{pack?.name ?? '加载中…'}</h1>
        <p>选择模组、锁定依赖,处理会阻塞交付的冲突。</p>
      </div>
      <span style={{display: 'inline-flex', gap: 8, alignItems: 'center'}}>
        <Button icon={<SettingOutlined/>} onClick={() => navigate('/settings')} title="导出目录白名单与平台凭证在全局设置里管理">全局设置</Button>
        <WorkbenchButton tone="primary" onClick={() => navigate(`/packs/${id}/publish`)} icon={<FileZipOutlined/>}>开始打包</WorkbenchButton>
      </span>
    </div>
    {error && <div className="empty-inline">加载失败:{error}</div>}
    <div className="workbench-grid">
      <main className="workbench-main">
        <CatalogSignalCard packId={id} phase={phase} itemCount={catalogItems.length}/>
        <WorkbenchCard className="mod-search-card">
          <div className="result-note">
            <span><strong className="tabular">{items.length}</strong> 个已选择模组</span>
            <Button type="link" onClick={() => navigate(`/packs/${id}/mods`)}>查看完整搜索 <ArrowRightOutlined/></Button>
          </div>
          {items.slice(0, 5).map(m => (
            <div className="mod-row mod-row-clickable" key={m.id}
              onClick={() => focusMod(m.canonicalModId || m.id, m.displayName, '概览')}
              title="点击在 Inspector 中聚焦此模组">
              <span className="mod-symbol"><CodeOutlined/></span>
              <div className="mod-row-main">
                <strong>{m.displayName}</strong>
                <span>{m.source} · {m.versionId || '—'}</span>
                <small>{m.fileName}</small>
              </div>
              <Tag color={m.status === 'disabled' ? 'gold' : 'green'}>{m.status}</Tag>
              <Button size="small" onClick={e => { e.stopPropagation(); navigate(`/packs/${id}/mods`); }}>查看</Button>
            </div>
          ))}
          {!error && items.length === 0 && <div className="empty-inline">当前还没有模组。去模组页搜索并添加。</div>}
          <Button block type="dashed" className="list-more" onClick={() => navigate(`/packs/${id}/mods`)}>搜索并添加更多模组</Button>
        </WorkbenchCard>
      </main>
      <PackHealthRail id={id}/>
    </div>
  </div>;
}

/* 概览的目录信号卡：把「物品大一统 / 合成器」的规模一眼摆在工作台首页，
   点任一格直达对应模块 —— 概览不再是死数字，而是进入链路的入口。 */
function CatalogSignalCard({packId, phase, itemCount}: {packId?: string; phase: string; itemCount: number}) {
  const navigate = useNavigate();
  const ready = phase === 'ready';
  const go = (suffix: string) => navigate(`/packs/${packId}${suffix}`);
  return <WorkbenchCard className="catalog-card overview-signals">
    <WorkbenchSectionHeader title="目录信号" action={<span className="db-muted">{ready ? '已构建' : phase === 'building' ? '构建中' : '未构建'}</span>}/>
    <div className="signal-grid">
      <button type="button" className="signal-tile" onClick={() => go('/items')} disabled={!ready}>
        <GoldOutlined/><b className="tabular">{ready ? itemCount : '—'}</b><span>物品</span>
      </button>
      <button type="button" className="signal-tile" onClick={() => go('/recipes')} disabled={!ready}>
        <ExperimentOutlined/><b className="tabular">{ready ? '浏览' : '—'}</b><span>合成器</span>
      </button>
      <button type="button" className="signal-tile" onClick={() => go('/tweak')}>
        <EditOutlined/><b className="tabular">魔改</b><span>内容文档</span>
      </button>
      <button type="button" className="signal-tile" onClick={() => go('/quests')}>
        <BookOutlined/><b className="tabular">任务</b><span>任务书</span>
      </button>
    </div>
  </WorkbenchCard>;
}

function PackHealthRail({id}: {id?: string}) {
  const navigate = useNavigate();
  const [health, setHealth] = useState<PackHealth | null>(null);
  const [locks, setLocks] = useState(0);
  const [pending, setPending] = useState(0);
  useEffect(() => {
    if (!id) return;
    void packHealth(id).then(setHealth).catch(() => setHealth(null));
    void listLocks(id).then(v => setLocks(v.length)).catch(() => setLocks(0));
    void listConflicts(id).then(v => setPending(v.filter(c => c.status !== 'resolved').length)).catch(() => setPending(0));
  }, [id]);
  const score = health
    ? (health.healthy ? 100 : Math.max(0, 100 - health.pendingErrors * 20 - health.pendingWarnings * 5))
    : null;
  return <aside className="pack-health-rail">
    <div className="health-kicker">PACK HEALTH</div>
    <h2>包健康</h2>
    <div className="health-score">
      {score === null ? <strong>—</strong> : <strong className="tabular">{score}</strong>}
      <span>/ 100</span>
      <Tag color={score === null ? 'default' : health?.healthy ? 'green' : 'gold'}>
        {score === null ? '加载中' : health?.healthy ? '健康' : '需要关注'}
      </Tag>
    </div>
    <div className="health-list">
      <div><CheckCircleFilled className="text-success"/><span>已安装模组</span><b className="tabular">{health?.installed ?? '—'}</b></div>
      <div><WarningFilled className={pending ? 'text-danger' : 'text-success'}/><span>待解决冲突</span><b className={`tabular ${pending ? 'text-danger' : ''}`}>{pending}</b></div>
      <div><InfoCircleOutlined className="text-info"/><span>锁定快照</span><b className="tabular">{locks}</b></div>
      <div><CheckOutlined className="text-success"/><span>模组总数</span><b className="tabular">{health?.mods ?? '—'}</b></div>
    </div>
    <Divider/>
    <Button block onClick={() => navigate(`/packs/${id}/dependencies`)}>处理冲突 <ArrowRightOutlined/></Button>
  </aside>;
}

function fmtDownloads(n?: number) { if (!n) return '0'; if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M'; if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K'; return String(n); }

const searchErrorText: Record<string, string> = {
  not_configured: '未配置(设置 CURSEFORGE_API_KEY 后可用)',
  rate_limited: '请求过快,稍后再试',
  unauthorized: 'API key 无效',
  unavailable: '平台暂时不可用',
  not_found: '资源不存在',
};

const platformTag = (pl: string) => <Tag key={pl} color={pl === 'modrinth' ? 'green' : pl === 'curseforge' ? 'orange' : 'default'}>{pl === 'modrinth' ? 'Modrinth' : pl === 'curseforge' ? 'CurseForge' : pl}</Tag>;

export function PackModsPage() {
  const {id} = useParams();
  const navigate = useNavigate();
  const {pack} = usePack(id);
  const s = useModSearch(id, pack);
  const {items} = usePackCatalog();
  const {focusMod} = useFocus();

  /* 命名空间 → 物品数：物品 id 前缀即贡献它的模组命名空间（minecraft: / create: …）。
     用于在已安装模组行给出「贡献 N 物品」的反向入口，点击直达过滤后的物品页。 */
  const itemsByNs = useMemo(() => {
    const m = new Map<string, number>();
    for (const it of items) {
      const ns = it.id.split(':')[0];
      if (ns) m.set(ns, (m.get(ns) ?? 0) + 1);
    }
    return m;
  }, [items]);

  const openModItems = (modId: string, label: string) => {
    focusMod(modId, label, '模组页');
    navigate(`/packs/${id}/items?ns=${encodeURIComponent(modId)}`);
  };

  return <div className="workspace-page">
    <div className="page-heading compact">
      <div>
        <span className="eyebrow">MOD CATALOG</span>
        <h1>模组</h1>
        <p>按名称同时搜索 Modrinth 和 CurseForge,版本按当前包 MC {pack?.mcVersion ?? '…'} · {pack?.loader ?? '…'} 过滤。</p>
      </div>
    </div>
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
          {[p.provider, ...(p.mirror ? [p.mirror.provider] : [])].map(platformTag)}
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
    {s.recommendations.length > 0 && <WorkbenchCard className="catalog-card">
      <WorkbenchSectionHeader title="推荐兼容模组(人工核实)"/>
      {s.recommendations.map(r => <div className="mod-row" key={r.projectId}>
        <span className="mod-symbol"><CodeOutlined/></span>
        <div className="mod-row-main"><strong>{r.name}</strong><small>{r.reason}</small></div>
        {platformTag(r.provider)}
        <Button size="small" loading={s.recBusy === r.projectId} onClick={() => s.addRecommendation(r)}>添加</Button>
      </div>)}
    </WorkbenchCard>}
    <WorkbenchCard className="catalog-card">
      <WorkbenchSectionHeader title={`已安装(${s.installed.length})`}/>
      {s.installed.map(m => <div className="mod-row" key={m.id}>
        <span className="mod-symbol"><CodeOutlined/></span>
        <div className="mod-row-main"><strong>{m.displayName}</strong><span>{m.source} · {m.versionId || '—'}</span><small>{m.fileName}</small></div>
        {[m.source, ...(m.mirrorSource ? [m.mirrorSource] : [])].filter(pl => pl === 'modrinth' || pl === 'curseforge').map(platformTag)}
        {m.origin === 'compat-fix' && <Tag color="blue">兼容补丁</Tag>}
        {!m.mirrorSource && (m.source === 'modrinth' || m.source === 'curseforge') && <Tag>仅单平台</Tag>}
        <Tag color={m.status === 'disabled' ? 'gold' : 'green'}>{m.status}</Tag>
        {(itemsByNs.get(m.canonicalModId) ?? 0) > 0 && (
          <Button size="small" type="link" icon={<GoldOutlined/>}
            onClick={() => openModItems(m.canonicalModId, m.displayName)}>
            贡献 {itemsByNs.get(m.canonicalModId)} 物品
          </Button>
        )}
        <Button size="small" danger={m.status !== 'disabled'} onClick={() => s.toggleInstalled(m)}>{m.status === 'disabled' ? '启用' : '移除'}</Button>
      </div>)}
      {!s.installed.length && <div className="empty-inline">当前还没有模组。搜索并添加第一个。</div>}
    </WorkbenchCard>
  </div>;
}

export function DependenciesPage() {
  const {id} = useParams();
  const {conflicts, locks, error, resolve} = useDependencies(id);
  /* 后端 /conflicts 返回的是全量(含 ignored/resolved),这里只呈现真正待处理的。
     之前「待处理项」把已忽略的冲突也列出来,与上方计数口径不一致。 */
  const pending = conflicts.filter(c => c.status === 'pending');
  const statusLabel: Record<string, string> = {pending: '待处理', ignored: '已忽略', resolved: '已解决'};
  return <div className="workspace-page">
    <div className="page-heading compact">
      <div>
        <span className="eyebrow">DEPENDENCIES / RESOLUTION</span>
        <h1>依赖与冲突</h1>
        <p>把版本问题变成明确的选择,解决后再进入发布检查。</p>
      </div>
      <WorkbenchButton tone="primary" icon={<CheckOutlined/>} onClick={resolve}>重新解析依赖</WorkbenchButton>
    </div>
    {error && <div className="empty-inline">加载失败:{error}</div>}
    {/* 本页没有右侧 rail(样式里的 .dependency-rail 未被使用),
        沿用两列栅格会在右边留出一条 320px 空白带 —— 用 solo 收成单列。 */}
    <div className="dependency-grid solo">
      <main>
        <div className="resolution-summary">
          <div><span>锁定快照</span><strong className="text-success tabular">{locks.length}</strong></div>
          <div><span>待处理冲突</span><strong className="text-danger tabular">{pending.length}</strong></div>
        </div>
        <WorkbenchCard className="conflict-card">
          <WorkbenchSectionHeader title="待处理项" action={<Button type="text" icon={<ReloadOutlined/>} onClick={resolve}>重新检查</Button>}/>
          {pending.map(c => <div className="conflict-row" key={c.id}>
            <span className={`conflict-mark ${c.severity === 'error' ? 'high' : 'medium'}`}><WarningFilled/></span>
            <div><strong>{c.summary}</strong><Tag color={c.severity === 'error' ? 'red' : 'gold'}>{c.kind}</Tag><p>{statusLabel[c.status] ?? c.status}</p></div>
          </div>)}
          {!error && !pending.length && <div className="resolved-state"><CheckCircleFilled/><strong>没有冲突</strong></div>}
        </WorkbenchCard>
      </main>
    </div>
  </div>;
}

export function ContentEditorPage() {
  const {id = ''} = useParams();
  const editor = useContentEditor(id);
  return <div className="workspace-page editor-page">
    <div className="page-heading compact">
      <div>
        <span className="eyebrow">CONTENT DOCUMENT</span>
        <h1>{editor.doc?.title ?? '内容文档'}</h1>
        <p>{editor.doc ? `${editor.doc.kind} · ${editor.doc.slug}` : '尚未选择内容文档。模组解析结果请到「内容编辑」页浏览。'}</p>
      </div>
    </div>
    <WorkbenchCard>
      {editor.error && <div className="empty-inline">加载失败:{editor.error}</div>}
      {!editor.error && !editor.doc && <div className="empty-inline">该包还没有内容文档。可在内容编辑页解析模组后查看。</div>}
      {editor.doc && (
        <>
          <p>当前修订：{editor.doc.activeRevisionId || '草稿'}</p>
          <span style={{display: 'inline-flex', gap: 8}}>
            <Button onClick={editor.validate}>校验</Button>
            <Button type="primary" onClick={editor.apply}>应用</Button>
          </span>
        </>
      )}
    </WorkbenchCard>
  </div>;
}

export function QuestEditorPage() {
  const {id = ''} = useParams();
  const quest = useQuestBook(id);

  if (quest.error) {
    return <div className="workspace-page">
      <div className="page-heading compact"><div><span className="eyebrow">QUEST BOOK</span><h1>任务书</h1></div></div>
      <WorkbenchCard>
        <div className="empty-inline">任务书加载失败：{quest.error}</div>
        <Button type="primary" onClick={() => void quest.reload()}>重试</Button>
      </WorkbenchCard>
    </div>;
  }

  if (quest.loading || !quest.book) {
    return <div className="workspace-page">
      <div className="page-heading compact"><div><span className="eyebrow">QUEST BOOK</span><h1>任务书</h1><p>正在准备任务书…</p></div></div>
      <WorkbenchCard><div className="empty-inline">加载中…</div></WorkbenchCard>
    </div>;
  }

  return <div className="workspace-page editor-page">
    <div className="page-heading compact qb-page-heading">
      <div>
        <span className="eyebrow">QUEST BOOK</span>
        <h1>{((quest.book.revision.draft as {book?: {title?: string}} | null)?.book?.title) || '任务书'}</h1>
        <p>
          章节 · 画布 · 依赖 · 预览；保存后校验，通过再应用。
        </p>
      </div>
      <div className="qb-rev-meta" data-testid="quest-rev-meta">
        <span className={`qb-state-tag state-${quest.book.revision.state}`}>
          {quest.book.revision.state === 'applied' ? '已应用' : quest.book.revision.state === 'draft' ? '草稿' : quest.book.revision.state}
        </span>
        <span className="qb-rev-no">修订 {quest.book.revision.revision || '—'}</span>
        {quest.lastAction && (
          <span className={`qb-last-action is-${quest.lastAction.status}`} title={quest.lastAction.text}>
            {quest.lastAction.text}
          </span>
        )}
        <span style={{display: 'inline-flex', gap: 8, alignItems: 'center'}}>
          <Button size="small" loading={quest.busy} onClick={() => void quest.reload()}>刷新</Button>
          <Button size="small" loading={quest.busy} onClick={() => void quest.validate()}>校验</Button>
          <WorkbenchButton size="small" tone="primary" loading={quest.busy} onClick={() => void quest.apply()}>应用</WorkbenchButton>
        </span>
      </div>
    </div>
    <QuestBookEditor
      packId={id}
      book={quest.book}
      busy={quest.busy}
      revision={quest.book.revision.revision}
      onSave={(draft, ifMatch) => {
        void quest.saveDraft(draft, ifMatch);
      }}
      onValidate={() => { void quest.validate(); }}
      onApply={() => { void quest.apply(); }}
    />
  </div>;
}

export function PublishPage() {
  const {id = ''} = useParams();
  const {message} = App.useApp();
  const [checks, setChecks] = useState<{kind: string; status: string; detail: string}[]>([]);
  const [versions, setVersions] = useState<{id: string; version: string}[]>([]);
  const [artifacts, setArtifacts] = useState<Artifact[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [exportDirName, setExportDirName] = useState('default-export');
  const [exportDirPath, setExportDirPath] = useState('');
  const [pickerOpen, setPickerOpen] = useState(false);
  const [exportReady, setExportReady] = useState(false);
  const [newVersion, setNewVersion] = useState('');
  const [newChannel, setNewChannel] = useState<'draft' | 'release'>('draft');

  const refresh = () => {
    void Promise.all([
      listDeliveryChecks(id).then(v => setChecks(v.items)),
      listVersions(id).then(setVersions),
      listArtifacts(id).then(setArtifacts),
    ]).catch(e => setError(e instanceof Error ? e.message : String(e)));
  };
  useEffect(refresh, [id]);

  const onRunChecks = async () => {
    const packVersionId = versions[0]?.id || '';
    if (!packVersionId) { message.error('暂无可用版本，无法重新检查'); return; }
    setBusy('checks');
    try {
      const res = await runDeliveryChecks(id, {packVersionId, checks: checks.length ? checks : [{kind: 'content', status: 'passed', detail: '{}'}]});
      setChecks(res.items ?? []);
      message.success('交付检查已更新');
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally { setBusy(''); }
  };

  const onRegisterExport = async () => {
    const path = exportDirPath.trim();
    if (!path) { message.error('请先选择或填写导出目录'); return; }
    setBusy('export');
    try {
      await registerExportDir(exportDirName.trim() || 'default-export', path);
      setExportReady(true);
      message.success('导出目录已注册，可以开始构建');
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // 同名目录冲突视为已注册，允许继续构建
      if (/conflict/i.test(msg)) {
        setExportReady(true);
        message.success('导出目录已存在，可直接构建');
      } else {
        setExportReady(false);
        message.error(msg);
      }
    } finally { setBusy(''); }
  };

  const onCreateVersion = async () => {
    const version = newVersion.trim();
    if (!version) { message.error('请填写版本号，例如 0.1.0'); return; }
    setBusy('version');
    try {
      await createVersion(id, {version, channel: newChannel});
      message.success(`版本 ${version} 已登记`);
      setNewVersion('');
      refresh();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally { setBusy(''); }
  };

  const onBuild = async () => {
    const packVersionId = versions[0]?.id;
    if (!packVersionId) { message.error('暂无版本，无法构建'); return; }
    if (!exportReady && !exportDirPath.trim()) { message.error('请先注册导出目录'); return; }
    setBusy('build');
    try {
      if (!exportReady && exportDirPath.trim()) {
        await registerExportDir(exportDirName.trim() || 'default-export', exportDirPath.trim()).catch(() => undefined);
      }
      const locks = await listLocks(id).catch(() => []);
      const lockSnapshot = locks[0]?.snapshot ? JSON.parse(locks[0].snapshot) : {packId: id, mods: []};
      // 这里以前由前端拼一个 {formatVersion:1} 的空 manifest 塞进 files[]，
      // 产物因此是不含任何模组的 283 字节空壳。files[] 省略即走后端装配：
      // 服务端从包内权威清单（pack_mods → 选中项 → 平台发布文件）生成真 modrinth.index.json。
      const res = await buildPack(id, {
        packVersionId,
        exportDirName: exportDirName.trim() || 'default-export',
        lockSnapshot,
      });
      message.success(`构建完成：${res.artifact?.fileName ?? 'artifact'}`);
      setExportReady(true);
      refresh();
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally { setBusy(''); }
  };

  return <div className="workspace-page">
    <div className="page-heading compact">
      <div>
        <span className="eyebrow">PUBLISH / DELIVERY</span>
        <h1>打包与发布</h1>
        <p>交付检查、导出目录与构建产物。构建前必须先注册允许的导出目录。</p>
      </div>
    </div>
    {error && <div className="empty-inline">加载失败:{error}</div>}
    <div className="release-layout">
      <main style={{display: 'flex', flexDirection: 'column', gap: 16}}>
        <WorkbenchCard className="release-check-card">
          <WorkbenchSectionHeader
            title="交付检查"
            action={<Button icon={<ReloadOutlined/>} loading={busy === 'checks'} onClick={onRunChecks}>重新检查</Button>}
          />
          {checks.map(c => (
            <div className="check-row" key={c.kind}>
              <CheckCircleFilled className={c.status === 'passed' ? 'check-icon' : ''} style={{color: c.status === 'passed' ? undefined : '#C9783B'}}/>
              <div>
                <strong>{c.kind}</strong>
                <span>{c.status} · {c.detail}</span>
              </div>
              <Tag color={c.status === 'passed' ? 'green' : 'gold'}>{c.status}</Tag>
            </div>
          ))}
          {!checks.length && <div className="empty-inline">暂无交付检查记录，点击「重新检查」生成。</div>}
        </WorkbenchCard>
        <WorkbenchCard className="artifact-card">
          <WorkbenchSectionHeader title="版本与产物"/>
          <div className="check-row">
            <div>
              <strong>已登记版本</strong>
              <span>{versions.length ? versions.map(v => `${v.version}`).join(', ') : '暂无版本'}</span>
            </div>
          </div>
          <div className="version-new-row">
            <Input value={newVersion} onChange={e => setNewVersion(e.target.value)} placeholder="新版本号，如 0.2.0" onPressEnter={onCreateVersion}/>
            <Select value={newChannel} onChange={v => setNewChannel(v)} style={{width: 108}}
              options={[{value: 'draft', label: '草稿'}, {value: 'release', label: '正式'}]}/>
            <Button loading={busy === 'version'} onClick={onCreateVersion}>登记版本</Button>
          </div>
          <p className="version-new-hint">构建与发布都按「最新登记的版本」执行，产物名里的版本号即它。</p>
          {artifacts.map(a => (
            <div className="artifact-row" key={a.id}>
              <FileZipOutlined/>
              <div><strong>{a.fileName}</strong><span className="tabular">{a.kind === 'mrpack' ? '整合包 .mrpack' : a.kind} · {formatBytes(a.sizeBytes)} · {a.sha256.slice(0, 16)}…</span></div>
            </div>
          ))}
          {!artifacts.length && <div className="empty-inline">还没有构建产物。注册导出目录后点击「开始构建」。</div>}
        </WorkbenchCard>
      </main>
      <aside className="target-rail">
        <WorkbenchSectionHeader title="构建目标"/>
        <p>导出目录需服务端注册并校验标记文件后，构建才会写入。</p>
        <label style={{display: 'block', marginBottom: 6, color: 'var(--mc-muted)', fontSize: 12}}>目录名称</label>
        <Input value={exportDirName} onChange={e => setExportDirName(e.target.value)} placeholder="default-export"/>
        <label style={{display: 'block', margin: '12px 0 6px', color: 'var(--mc-muted)', fontSize: 12}}>导出路径</label>
        <div style={{display: 'flex', gap: 8}}>
          <Input value={exportDirPath} onChange={e => setExportDirPath(e.target.value)} placeholder="选择本机目录"/>
          <Button onClick={() => setPickerOpen(true)}>选择</Button>
        </div>
        <div style={{display: 'flex', flexDirection: 'column', gap: 10, marginTop: 16}}>
          <Button loading={busy === 'export'} onClick={onRegisterExport}>{exportReady ? '重新注册导出目录' : '注册导出目录'}</Button>
          <WorkbenchButton tone="primary" loading={busy === 'build'} onClick={onBuild}>开始构建</WorkbenchButton>
        </div>
        {exportReady && <div className="rail-note">导出目录已就绪，可开始构建。</div>}
      </aside>
    </div>
    <DirectoryPicker
      open={pickerOpen}
      title="选择导出目录"
      value={exportDirPath}
      onClose={() => setPickerOpen(false)}
      onSelect={setExportDirPath}
    />
  </div>;
}

const providerStatusText: Record<string, {label: string; color: string}> = {
  ok: {label: '已连接', color: 'green'},
  unavailable: {label: '不可用', color: 'red'},
  unknown: {label: '未探测', color: 'default'},
};

export function SettingsPage() {
  const {message} = App.useApp();
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [error, setError] = useState('');
  const [cfKey, setCfKey] = useState('');
  const [keyBusy, setKeyBusy] = useState(false);
  const [exportDirName, setExportDirName] = useState('settings-export');
  const [exportDirPath, setExportDirPath] = useState('');
  const [pickerOpen, setPickerOpen] = useState(false);
  const [exportBusy, setExportBusy] = useState(false);

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
  const onRegisterExport = async () => {
    if (!exportDirPath.trim()) { message.error('请选择导出目录'); return; }
    setExportBusy(true);
    try {
      await registerExportDir(exportDirName.trim() || 'settings-export', exportDirPath.trim());
      message.success('导出目录已注册');
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally { setExportBusy(false); }
  };

  return <div className="workspace-page settings-page">
    <div className="page-heading">
      <div>
        <span className="eyebrow">WORKSPACE / PREFERENCES</span>
        <h1>设置</h1>
        <p>平台连接状态、本地存储占用与构建导出目录。</p>
      </div>
    </div>
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
        <section className="settings-section">
          <WorkbenchSectionHeader title="构建导出目录"/>
          <div className="form-grid">
            <div>
              <label style={{display: 'block', marginBottom: 6, color: 'var(--mc-muted)', fontSize: 12}}>目录名称</label>
              <Input value={exportDirName} onChange={e => setExportDirName(e.target.value)}/>
            </div>
            <div>
              <label style={{display: 'block', marginBottom: 6, color: 'var(--mc-muted)', fontSize: 12}}>绝对路径</label>
              <div style={{display: 'flex', gap: 8}}>
                <Input value={exportDirPath} onChange={e => setExportDirPath(e.target.value)} placeholder="选择本机目录"/>
                <Button onClick={() => setPickerOpen(true)}>选择</Button>
              </div>
            </div>
          </div>
          <Button type="primary" style={{marginTop: 12}} loading={exportBusy} onClick={onRegisterExport}>注册导出目录</Button>
        </section>
      </main>
    </div>
    <DirectoryPicker open={pickerOpen} title="选择导出目录" value={exportDirPath} onClose={() => setPickerOpen(false)} onSelect={setExportDirPath}/>
  </div>;
}

function ProviderCard({name, status, reachable}: {name: string; status: 'unknown' | 'ok' | 'unavailable'; reachable: boolean}) {
  const s = providerStatusText[status] ?? providerStatusText.unknown;
  return <div className="provider-card"><span className="provider-mark"><LinkOutlined/></span><div><strong>{name}</strong><span>{reachable ? 'API 可达' : 'API 不可达或尚未探测'}</span></div><Tag color={s.color}>{s.label}</Tag></div>;
}
