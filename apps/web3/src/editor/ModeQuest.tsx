import {useEffect, useMemo, useState} from 'react';
import {
  applyQuest, getQuest, saveQuestDraft, validateQuest,
  type QuestBook, type QuestNode,
} from '../api/content';
import {useUrlPatch, useUrlState} from '../app/url';
import {ApiError} from '../api/http';
import {Icon} from '../ui/Icon';
import {ItemIdInput} from '../app/ItemIdInput';
import {chapterColor, emptyQuestDraft, rid, type QuestDraft} from './questDraft';
import {QuestCanvas} from './QuestCanvas';

/* 编排态（任务书）：中间是画布（和 FTB Quests 同一套交互），右边是选中任务的检查器。
   章节列表**不在这里** —— 它在左栏的章节面板里，一屏里不该有两个列表。

   画布的坐标是网格单位（FTB 的 doubles），后端 `x REAL / y REAL` 就是这个意思。

   草稿仍是本地的：点「保存草稿」才落库，If-Match 带当前 revision，别处先存过就会失败并提示，
   而不是静默覆盖。 */
type Draft = QuestDraft;

const ALL = '__all__';        // qscope=all 时的画布范围标识

export function ModeQuest() {
  const {packId, node: selectedId, chap, qscope, item: lastItem} = useUrlState();
  const patch = useUrlPatch();
  const [book, setBook] = useState<QuestBook | null>(null);
  const [hasBook, setHasBook] = useState(false);
  const [draft, setDraft] = useState<Draft>(emptyQuestDraft);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => {
    if (!packId) return;
    getQuest(packId)
      .then(b => {
        setBook(b); setHasBook(true);
        const d = b.revision.draft;
        setDraft({chapters: d.chapters, nodes: d.nodes, edges: d.edges});
        setError(null);
      })
      .catch(e => {
        /* 「还没有任务书」是后端的 404，属于正常空态而不是错误。 */
        setHasBook(false);
        if (e instanceof ApiError && e.status === 404) { setError(null); return; }
        setError(e instanceof Error ? e.message : String(e));
      });
  };
  useEffect(load, [packId]);

  const mut = (fn: (d: Draft) => Draft) => { setNotice(null); setDraft(fn); };

  const chapters = useMemo(
    () => [...draft.chapters].sort((a, b) => a.position - b.position),
    [draft.chapters]);

  /* 当前章节由 ?chap= 派生，不再用本地 state —— 左栏章节面板点一章写的就是它，
     两处共用一个事实源，就不会「左栏点的是第三章、编辑区还停在第一章」。 */
  const activeChapter = chap && chapters.some(c => c.id === chap)
    ? chap
    : (chapters[0]?.id ?? '');

  const showAll = qscope === 'all';

  const addChapter = () => {
    const id = rid('ch');
    mut(d => ({...d, chapters: [...d.chapters, {id, title: `第${d.chapters.length + 1}章`, description: '', coverColor: '', position: d.chapters.length}]}));
    patch({chap: id, qscope: null});
  };

  const addNodeAt = (x: number, y: number) => {
    if (chapters.length === 0) { addChapter(); return; }
    const ch = showAll ? activeChapter : activeChapter;
    const id = rid('node');
    mut(d => ({...d, nodes: [...d.nodes, {
      id, chapterId: ch, title: `新任务 ${d.nodes.length + 1}`, description: '', icon: '',
      x, y, prerequisites: [], rewards: [], modRefs: [], position: d.nodes.length,
    }]}));
    patch({node: id});
  };

  /* 工具栏的「+ 任务」没有落点，就摆在这一章最右一列的右边 —— 比随手丢在原点好找。 */
  const addNodeAuto = () => {
    const peers = showAll ? draft.nodes : draft.nodes.filter(n => n.chapterId === activeChapter);
    if (peers.length === 0) { addNodeAt(0, 0); return; }
    addNodeAt(Math.max(...peers.map(n => n.x)) + 3, Math.min(...peers.map(n => n.y)));
  };

  const patchNode = (id: string, next: Partial<QuestNode>) =>
    mut(d => ({...d, nodes: d.nodes.map(n => n.id === id ? {...n, ...next} : n)}));

  const removeNode = (id: string) => {
    mut(d => ({
      ...d,
      nodes: d.nodes.filter(n => n.id !== id),
      edges: d.edges.filter(e => e.fromNodeId !== id && e.toNodeId !== id),
    }));
    if (selectedId === id) patch({node: null});
  };

  /* 前置 = 指向本节点的边。勾选即增边，取消即删边，权威源始终是 edges。 */
  const setPrereq = (nodeId: string, fromId: string, on: boolean) =>
    mut(d => ({
      ...d,
      edges: on
        ? [...d.edges.filter(e => !(e.toNodeId === nodeId && e.fromNodeId === fromId)), {id: rid('edge'), fromNodeId: fromId, toNodeId: nodeId}]
        : d.edges.filter(e => !(e.toNodeId === nodeId && e.fromNodeId === fromId)),
    }));

  const save = async () => {
    if (!packId || !book) return;
    setBusy(true); setError(null); setNotice(null);
    try {
      const rev = await saveQuestDraft(packId, book.revision.revision, {...draft, book: book.revision.draft.book});
      setNotice(`已保存草稿 r${rev.revision}`);
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const run = async (fn: (packId: string) => Promise<{status: string; issues: {severity: string; message: string}[]}>, label: string) => {
    if (!packId) return;
    setBusy(true); setError(null); setNotice(null);
    try {
      const r = await fn(packId);
      const bad = r.issues.filter(i => i.severity === 'error').length;
      setNotice(`${label}：${r.status}${r.issues.length ? ` · ${r.issues.length} 条问题（error ${bad}）` : ''}`
        + (r.issues.length ? ` — ${r.issues.slice(0, 2).map(i => i.message).join('；')}` : ''));
      load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally { setBusy(false); }
  };

  const visible = useMemo(
    () => (showAll ? draft.nodes : draft.nodes.filter(n => n.chapterId === activeChapter)),
    [draft.nodes, showAll, activeChapter]);
  const visibleIds = useMemo(() => new Set(visible.map(n => n.id)), [visible]);
  /* 只画两端都看得见的边 —— 单章视角下一头在别的章节的线画出来是悬空的。 */
  const edges = useMemo(
    () => draft.edges.filter(e => visibleIds.has(e.fromNodeId) && visibleIds.has(e.toNodeId)),
    [draft.edges, visibleIds]);

  const selected = useMemo(() => draft.nodes.find(n => n.id === selectedId) ?? null, [draft.nodes, selectedId]);
  const selectedPrereqs = useMemo(
    () => new Set(draft.edges.filter(e => e.toNodeId === selectedId).map(e => e.fromNodeId)),
    [draft.edges, selectedId]);

  if (!packId) return null;

  const chapterTitle = (id: string) => chapters.find(c => c.id === id)?.title || id;
  const iconUrl = (itemId: string) =>
    `/api/packs/${encodeURIComponent(packId)}/catalog/icon?itemId=${encodeURIComponent(itemId)}`;

  return (
    <>
      <div className="ed-toolbar">
        <span className="title">任务书</span>
        {hasBook && (
          <>
            <select className="p-input" style={{width: 148}} value={showAll ? ALL : activeChapter}
              title="当前编辑的章节"
              onChange={e => patch(e.target.value === ALL
                ? {qscope: 'all', node: null}
                : {chap: e.target.value, qscope: null, node: null})}>
              {chapters.map(c => (
                <option key={c.id} value={c.id}>
                  {c.title || '(未命名章节)'}（{draft.nodes.filter(n => n.chapterId === c.id).length}）
                </option>
              ))}
              <option value={ALL} title="一次显示整本书；跨章节的前置连线只有在这里看得见">全部章节</option>
            </select>
            <button type="button" className="p-btn" title="新建章节并切过去"
              disabled={busy} onClick={addChapter}><Icon name="plus" size={12}/> 章节</button>
            <span className="sub"
              title={book ? `草稿修订 r${book.revision.revision}（并发控制用，保存时作 If-Match 令牌）` : ''}>
              任务 {visible.length}{showAll ? ` / ${draft.nodes.length}` : ''}
            </span>
          </>
        )}
        <span className="grow"/>
        {hasBook && (
          <>
            <button type="button" className="p-btn" disabled={busy} onClick={addNodeAuto}>
              <Icon name="plus" size={12}/> 任务
            </button>
            <button type="button" className="p-btn" disabled={busy} onClick={() => void save()}>保存草稿</button>
            <button type="button" className="p-btn" disabled={busy} onClick={() => void run(validateQuest, '校验')}>校验</button>
            <button type="button" className="p-btn primary" disabled={busy} onClick={() => void run(applyQuest, '应用')}>应用</button>
          </>
        )}
      </div>

      {error && <div className="p-empty" style={{color: 'var(--mc-fail)', margin: '6px 12px 0'}}>{error}</div>}
      {notice && <div className="p-empty" style={{color: 'var(--mc-success)', margin: '6px 12px 0'}}>{notice}</div>}

      {!hasBook && !error && (
        <div className="ed-scroll">
          <div className="ed-placeholder">
            任务书还是空的。建一份草稿后就能在画布上加任务、连前置。
            <div style={{marginTop: 8}}>
              <button type="button" className="p-btn primary" disabled={busy}
                onClick={async () => {
                  setBusy(true); setError(null);
                  try {
                    await saveQuestDraft(packId, 0, {...emptyQuestDraft(), book: {title: '任务书'}});
                    load();
                  } catch (e) {
                    setError(e instanceof Error ? e.message : String(e));
                  } finally { setBusy(false); }
                }}>创建任务书草稿</button>
            </div>
          </div>
        </div>
      )}

      {hasBook && (
        <div className="quest-board">
          <QuestCanvas
            nodes={visible}
            edges={edges}
            selectedId={selectedId}
            scopeKey={showAll ? ALL : activeChapter}
            colorOf={n => chapterColor(chapters, n.chapterId)}
            iconOf={n => (n.icon ? iconUrl(n.icon) : null)}
            onSelect={id => patch({node: id})}
            onMove={(id, x, y) => patchNode(id, {x, y})}
            onConnect={(from, to) => setPrereq(to, from, true)}
            onCreate={addNodeAt}
            onDelete={removeNode}/>

          {selected && (
            <aside className="quest-inspector">
              <div className="qi-head">
                <span className="p-title" style={{margin: 0}}>任务</span>
                <span className="grow"/>
                <button type="button" className="tp-icon-btn" title="删除这个任务"
                  onClick={() => removeNode(selected.id)}><Icon name="trash" size={13}/></button>
              </div>

              <label className="q-field"><span>标题</span>
                {/* 不 autoFocus：一旦抢焦点，Delete 键删除任务的快捷键
                    就会被 typing 守卫吞掉（useHotkeys / QuestCanvas 同一口径）。 */}
                <input className="p-input" value={selected.title}
                  onChange={e => patchNode(selected.id, {title: e.target.value})}/></label>
              <label className="q-field"><span>副标题</span>
                <input className="p-input" value={selected.subtitle ?? ''}
                  onChange={e => patchNode(selected.id, {subtitle: e.target.value})}/></label>
              <label className="q-field"><span>描述</span>
                <textarea className="p-input" rows={3} value={selected.description}
                  onChange={e => patchNode(selected.id, {description: e.target.value})}/></label>

              <label className="q-field"><span>图标（物品 id）</span>
                <ItemIdInput value={selected.icon} placeholder="minecraft:stone"
                  onChange={v => patchNode(selected.id, {icon: v})}/></label>
              <div className="qi-icon-row">
                {selected.icon
                  ? <img src={iconUrl(selected.icon)} alt="" className="qi-icon"/>
                  : <span className="qi-icon qi-icon-ph">?</span>}
                {lastItem && lastItem !== selected.icon && (
                  <button type="button" className="p-btn"
                    title={`把图标设成索引里选中的 ${lastItem}`}
                    onClick={() => patchNode(selected.id, {icon: lastItem})}>
                    用索引选中的物品
                  </button>
                )}
              </div>

              <div className="qi-grid">
                <label className="q-field"><span>章节</span>
                  <select className="p-input" value={selected.chapterId}
                    onChange={e => patchNode(selected.id, {chapterId: e.target.value})}>
                    {chapters.map(c => <option key={c.id} value={c.id}>{c.title || c.id}</option>)}
                  </select></label>
                <label className="q-field"><span>尺寸</span>
                  <select className="p-input" value={String(selected.size ?? 2)}
                    onChange={e => patchNode(selected.id, {size: Number(e.target.value)})}>
                    {[1, 1.5, 2, 2.5, 3].map(v => <option key={v} value={v}>{v}</option>)}
                  </select></label>
              </div>

              <label className="q-field"><span>形状</span>
                <select className="p-input" value={selected.shape ?? ''}
                  onChange={e => patchNode(selected.id, {shape: e.target.value || undefined})}>
                  <option value="">默认（圆角方）</option>
                  <option value="square">方形</option>
                  <option value="circle">圆形</option>
                </select></label>

              <div className="qi-checks">
                <label className="q-check">
                  <input type="checkbox" checked={selected.optional ?? false}
                    onChange={e => patchNode(selected.id, {optional: e.target.checked})}/>
                  <span>可选任务（不挡住主线）</span>
                </label>
                <label className="q-check">
                  <input type="checkbox" checked={selected.invisible ?? false}
                    onChange={e => patchNode(selected.id, {invisible: e.target.checked})}/>
                  <span>隐藏（画布上淡出）</span>
                </label>
              </div>

              <div className="qi-head" style={{marginTop: 10}}>
                <span className="p-title" style={{margin: 0}}>前置任务 <span className="count">{selectedPrereqs.size}</span></span>
                <span className="grow"/>
                {selectedPrereqs.size > 0 && (
                  <button type="button" className="tp-icon-btn" title="清空全部前置"
                    onClick={() => mut(d => ({...d, edges: d.edges.filter(e => e.toNodeId !== selected.id)}))}>
                    <Icon name="trash" size={13}/>
                  </button>
                )}
              </div>
              <div className="q-prereq">
                {draft.nodes.filter(n => n.id !== selected.id).map(n => (
                  <label key={n.id} className="q-check">
                    <input type="checkbox" checked={selectedPrereqs.has(n.id)}
                      onChange={e => setPrereq(selected.id, n.id, e.target.checked)}/>
                    <span className="qi-prereq-name">{n.title || n.id}</span>
                    <span className="sub">{chapterTitle(n.chapterId)}</span>
                  </label>
                ))}
                {draft.nodes.length <= 1 && <div className="p-empty">还没有别的任务可以当前置。</div>}
              </div>
              <div className="q-field-hint">
                前置写入 draft.edges；画布上拖节点右缘的圆点到另一个节点，效果一样。
                保存后由后端同步为节点的 prerequisites。
              </div>

              <div className="qi-head" style={{marginTop: 10}}>
                <span className="p-title" style={{margin: 0}}>任务与奖励</span>
              </div>
              <div className="q-field-hint">
                当前 {selected.tasks?.length ?? 0} 条任务、{selected.rewards.length} 条奖励。
                这两块是自由结构（后端只做透传与校验），编辑器还没做 ——
                现在改了也只能靠手写 JSON，先别当真。
              </div>
            </aside>
          )}
        </div>
      )}
    </>
  );
}
