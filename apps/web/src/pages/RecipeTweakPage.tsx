import {useCallback, useEffect, useMemo, useState} from 'react';
import {useParams} from 'react-router-dom';
import {App, Button, Drawer, Empty, Input, Modal, Select, Spin, Tag} from 'antd';
import {
  EditOutlined, PlusOutlined, ReloadOutlined, SaveOutlined,
  CheckOutlined, ThunderboltOutlined, HistoryOutlined,
} from '@ant-design/icons';
import {WorkbenchButton, WorkbenchCard, WorkbenchSectionHeader} from '../ui/workbench/Workbench';
import {
  applyContent, contentHistory, createContent, getContent, listContent,
  rollbackContent, saveContentDraft, validateContent,
  type ContentDocument, type ContentRevision, type ContentValidation,
} from '../api/content';
import {ApiError} from '../api/http';
import {RecipeViewer} from '../features/recipe/RecipeViewer';
import {usePackCatalog} from '../features/pack/PackCatalogContext';
import {useFocus} from '../features/focus/FocusContext';
import './recipe-tweak.css';

/* 魔改页（写态）：内容文档 = 可编辑的配方/结构/矿脉。
   与「合成器」(读态) 是同一配方对象的两种状态 —— 这里改 payload JSON，右侧用共享目录实时预览 JEI 网格，
   改动即所见。走后端修订协议：存草稿(If-Match 乐观锁) → 校验 → 应用，支持历史与回滚。 */

const KIND_OPTIONS = [
  {value: 'recipe', label: '配方 recipe'},
  {value: 'structure', label: '结构 structure'},
  {value: 'ore', label: '矿脉 ore'},
];

const RECIPE_TEMPLATE = {
  type: 'minecraft:crafting_shaped',
  pattern: ['XXX', 'X#X', 'XXX'],
  key: {X: {item: 'minecraft:stone'}, '#': {item: 'minecraft:stick'}},
  result: {id: 'minecraft:stone_bricks', count: 1},
};

const defaultPayload = (kind: string) => (kind === 'recipe' ? RECIPE_TEMPLATE : {});

export function RecipeTweakPage() {
  const {id = ''} = useParams();
  const {message} = App.useApp();
  const {displayName, iconUrl, itemById} = usePackCatalog();
  const {focusItem} = useFocus();

  const [docs, setDocs] = useState<ContentDocument[]>([]);
  const [kindFilter, setKindFilter] = useState<string>('recipe');
  const [loadingDocs, setLoadingDocs] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<{document: ContentDocument; revision: ContentRevision | null} | null>(null);
  const [payloadText, setPayloadText] = useState('');
  const [busy, setBusy] = useState(false);
  const [validation, setValidation] = useState<ContentValidation | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [history, setHistory] = useState<ContentRevision[]>([]);
  const [form, setForm] = useState({kind: 'recipe', slug: '', title: ''});

  const loadDocs = useCallback(async () => {
    setLoadingDocs(true);
    try {
      const list = await listContent(id, kindFilter || undefined);
      setDocs(list);
      if (list.length && !list.some(d => d.id === selectedId)) setSelectedId(list[0].id);
      if (!list.length) { setSelectedId(null); setDetail(null); }
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setLoadingDocs(false);
    }
  }, [id, kindFilter, selectedId, message]);

  useEffect(() => { void loadDocs(); }, [id, kindFilter]); // eslint-disable-line react-hooks/exhaustive-deps

  const openDoc = useCallback(async (docId: string) => {
    setSelectedId(docId);
    setValidation(null);
    try {
      const d = await getContent(id, docId);
      setDetail(d);
      const payload = d.revision?.payload ?? defaultPayload(d.document.kind);
      setPayloadText(JSON.stringify(payload, null, 2));
    } catch (e) {
      message.error(e instanceof Error ? e.message : String(e));
    }
  }, [id, message]);

  useEffect(() => { if (selectedId) void openDoc(selectedId); }, [selectedId]); // eslint-disable-line react-hooks/exhaustive-deps

  /* payload 文本 → 对象；解析错误就地展示，不在渲染期 setState。 */
  const {parsedPayload, jsonError} = useMemo(() => {
    try { return {parsedPayload: JSON.parse(payloadText) as unknown, jsonError: ''}; }
    catch (e) { return {parsedPayload: null, jsonError: e instanceof Error ? e.message : String(e)}; }
  }, [payloadText]);

  const getItemIcon = useCallback((k: string): string | null => {
    if (!k || k.startsWith('#')) return null;
    const it = itemById.get(k);
    return it && it.iconStatus === 'ready' ? iconUrl(k) : null;
  }, [itemById, iconUrl]);

  const revisionNo = detail?.revision?.revision ?? 0;
  const state = detail?.revision?.state ?? 'draft';

  const doCreate = async () => {
    if (!form.slug.trim() || !form.title.trim()) { message.warning('slug 与标题必填'); return; }
    setBusy(true);
    try {
      const doc = await createContent(id, {kind: form.kind, slug: form.slug.trim(), title: form.title.trim()});
      message.success('已新建内容文档');
      setCreateOpen(false);
      setForm({kind: 'recipe', slug: '', title: ''});
      await loadDocs();
      setSelectedId(doc.id);
    } catch (e) {
      message.error(e instanceof ApiError ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const doSave = async () => {
    if (!detail || parsedPayload === null) { message.error('payload 不是合法 JSON，无法保存'); return; }
    setBusy(true);
    try {
      const rev = await saveContentDraft(id, detail.document.id, revisionNo, parsedPayload);
      setDetail({document: detail.document, revision: rev});
      setPayloadText(JSON.stringify(rev.payload ?? parsedPayload, null, 2));
      message.success('草稿已保存');
    } catch (e) {
      if (e instanceof ApiError && (e.code === 'revision_conflict' || e.status === 412 || e.status === 409)) {
        message.error('修订冲突：内容已被修改，请重新打开文档'); void openDoc(detail.document.id);
      } else message.error(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const doValidate = async () => {
    if (!detail) return;
    setBusy(true);
    try {
      const v = await validateContent(id, detail.document.id);
      setValidation(v);
      if (v.status === 'failed') message.error(`校验未通过（${v.issues.length}）`);
      else if (v.issues.length) message.warning(`校验有 ${v.issues.length} 条提示`);
      else message.success('校验通过');
    } catch (e) { message.error(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  };

  const doApply = async () => {
    if (!detail) return;
    setBusy(true);
    try {
      const r = await applyContent(id, detail.document.id);
      message.success(`已应用（${r.status}）`);
      await openDoc(detail.document.id);
    } catch (e) {
      if (e instanceof ApiError && e.code === 'content_not_validated') message.error('请先校验通过再应用');
      else message.error(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const openHistory = async () => {
    if (!detail) return;
    try { const h = await contentHistory(id, detail.document.id); setHistory(h); setHistoryOpen(true); }
    catch (e) { message.error(e instanceof Error ? e.message : String(e)); }
  };

  const doRollback = async (revId: string) => {
    if (!detail) return;
    setBusy(true);
    try {
      const rev = await rollbackContent(id, detail.document.id, revId);
      message.success('已回滚（生成新修订）');
      setDetail({document: detail.document, revision: rev});
      setPayloadText(JSON.stringify(rev.payload ?? {}, null, 2));
      setHistoryOpen(false);
    } catch (e) { message.error(e instanceof Error ? e.message : String(e)); }
    finally { setBusy(false); }
  };

  const isRecipe = detail?.document.kind === 'recipe';

  return (
    <div className="workspace-page">
      <div className="page-heading compact">
        <div>
          <span className="eyebrow">RECIPE TWEAK</span>
          <h1>魔改</h1>
          <p>编辑配方 / 结构 / 矿脉内容文档。改 payload，右侧实时预览合成网格 —— 读态（合成器）与写态（这里）是同一配方的两面。</p>
        </div>
        <span style={{display: 'inline-flex', gap: 8}}>
          <Button icon={<ReloadOutlined/>} onClick={() => void loadDocs()}>刷新</Button>
          <WorkbenchButton tone="primary" icon={<PlusOutlined/>} onClick={() => setCreateOpen(true)}>新建内容文档</WorkbenchButton>
        </span>
      </div>

      <div className="rt-layout">
        <WorkbenchCard className="catalog-card rt-list">
          <WorkbenchSectionHeader
            title={`内容文档 (${docs.length})`}
            action={
              <Select size="small" value={kindFilter} onChange={setKindFilter} style={{width: 130}}
                options={[{value: '', label: '全部'}, ...KIND_OPTIONS]}/>
            }
          />
          {loadingDocs ? <div className="empty-inline"><Spin/></div>
            : docs.length ? docs.map(d => (
              <button key={d.id} className={`rt-doc ${d.id === selectedId ? 'active' : ''}`} onClick={() => setSelectedId(d.id)}>
                <EditOutlined/>
                <span className="rt-doc-main">
                  <strong>{d.title}</strong>
                  <small>{d.kind} · {d.slug}</small>
                </span>
              </button>
            ))
            : <div className="empty-inline">还没有内容文档。点「新建内容文档」开始魔改第一个配方。</div>}
        </WorkbenchCard>

        <main className="rt-main">
          {!detail ? (
            <WorkbenchCard className="catalog-card rt-empty">
              <Empty description="选择左侧一个内容文档，或新建一个。"/>
            </WorkbenchCard>
          ) : (
            <>
              <WorkbenchCard className="catalog-card rt-editor">
                <div className="rt-editor-head">
                  <div>
                    <strong>{detail.document.title}</strong>
                    <div className="rt-editor-sub">
                      <Tag>{detail.document.kind}</Tag>
                      <code>{detail.document.slug}</code>
                      <Tag color={state === 'applied' ? 'green' : 'gold'}>{state}</Tag>
                      <span className="db-muted">修订 {revisionNo}</span>
                    </div>
                  </div>
                  <div className="rt-editor-actions">
                    <Button size="small" icon={<SaveOutlined/>} loading={busy} onClick={() => void doSave()}>存草稿</Button>
                    <Button size="small" icon={<CheckOutlined/>} loading={busy} onClick={() => void doValidate()}>校验</Button>
                    <WorkbenchButton size="small" tone="primary" icon={<ThunderboltOutlined/>} loading={busy} onClick={() => void doApply()}>应用</WorkbenchButton>
                    <Button size="small" icon={<HistoryOutlined/>} onClick={() => void openHistory()}>历史</Button>
                  </div>
                </div>

                <div className="rt-editor-body">
                  <div className="rt-payload">
                    <label>payload (JSON)</label>
                    <Input.TextArea
                      value={payloadText}
                      onChange={e => setPayloadText(e.target.value)}
                      autoSize={{minRows: 16, maxRows: 30}}
                      spellCheck={false}
                      style={{fontFamily: 'ui-monospace, monospace', fontSize: 12}}
                    />
                    {jsonError && <div className="rt-json-error">JSON 解析失败：{jsonError}</div>}
                  </div>

                  <div className="rt-preview">
                    <label>实时预览</label>
                    {isRecipe
                      ? parsedPayload !== null
                        ? <RecipeViewer payload={parsedPayload} translateKey={displayName} getItemIcon={getItemIcon} onSelect={iid => !iid.startsWith('#') && focusItem(iid, '魔改预览')}/>
                        : <div className="empty-inline">修正 JSON 后预览</div>
                      : <div className="empty-inline">该类型（{detail.document.kind}）暂无图形预览，直接编辑 payload。</div>}
                  </div>
                </div>

                {validation && (
                  <div className={`rt-validation ${validation.status}`}>
                    <strong>校验：{validation.status}</strong>
                    {validation.issues.length
                      ? <ul>{validation.issues.map((iss, i) => <li key={i}>[{iss.severity}] {iss.code}{iss.path ? ` @${iss.path}` : ''}：{iss.message}</li>)}</ul>
                      : <span className="db-muted">无问题</span>}
                  </div>
                )}
              </WorkbenchCard>
            </>
          )}
        </main>
      </div>

      <Modal
        open={createOpen}
        title="新建内容文档"
        onCancel={() => setCreateOpen(false)}
        onOk={() => void doCreate()}
        confirmLoading={busy}
        okText="创建"
      >
        <div className="rt-form">
          <label>类型</label>
          <Select value={form.kind} onChange={v => setForm(f => ({...f, kind: v}))} options={KIND_OPTIONS} style={{width: '100%'}}/>
          <label>slug（同包内唯一）</label>
          <Input value={form.slug} onChange={e => setForm(f => ({...f, slug: e.target.value}))} placeholder="custom_iron_recipe"/>
          <label>标题</label>
          <Input value={form.title} onChange={e => setForm(f => ({...f, title: e.target.value}))} placeholder="自定义铁锭配方"/>
        </div>
      </Modal>

      <Drawer title="修订历史" open={historyOpen} onClose={() => setHistoryOpen(false)} width={420}>
        {history.length ? history.map(r => (
          <div className="rt-history-row" key={r.id}>
            <div>
              <strong>修订 {r.revision}</strong> <Tag color={r.state === 'applied' ? 'green' : 'default'}>{r.state}</Tag>
              <div className="db-muted" style={{fontSize: 12}}>{new Date(r.createdAt).toLocaleString()}</div>
            </div>
            <Button size="small" onClick={() => void doRollback(r.id)}>回滚到此</Button>
          </div>
        )) : <Empty description="暂无历史修订"/>}
      </Drawer>
    </div>
  );
}
