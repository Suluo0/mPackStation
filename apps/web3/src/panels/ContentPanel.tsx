import {useCallback, useEffect, useState} from 'react';
import {createContent, listContent, type ContentDocument} from '../api/content';
import {useUrlPatch, useUrlState} from '../app/url';
import {Button} from '../ui/Button';
import {Prompt} from '../ui/Prompt';

/* 自建内容管理：按种类列文档 + 新建。

   种类**不是随便列的**：后端 validContentKind 只认 recipe / structure / ore
   （apps/server/internal/service/content.go），传别的会被 422 拒。
   所以这张表就是后端的枚举，后端扩了才往里加。 */
const KINDS: Record<string, string> = {
  recipe: '配方',
  structure: '结构',
  ore: '矿脉',
};

export function ContentPanel() {
  const {packId, ck} = useUrlState();
  const patch = useUrlPatch();
  const kind = ck ?? 'recipe';
  const [docs, setDocs] = useState<ContentDocument[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const load = useCallback(() => {
    if (!packId) return;
    listContent(packId, kind)
      .then(setDocs)
      .catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId, kind]);
  useEffect(load, [load]);

  if (!packId) return null;

  return (
    <div className="tp-body">
      {/* 种类切换（2026-10-04 用户反馈：后端支持配方/结构/矿脉三种，但此前
          ?ck= 没有 UI 写入点，结构/矿脉成了藏在 URL 后面的功能）。 */}
      <div className="p-section">
        <div className="p-title">自建内容</div>
        <div className="ck-tabs">
          {Object.entries(KINDS).map(([value, label]) => (
            <button key={value} type="button"
              className={`ck-tab${kind === value ? ' on' : ''}`}
              aria-pressed={kind === value}
              onClick={() => patch({ck: value === 'recipe' ? null : value, doc: null})}>
              {label}
            </button>
          ))}
        </div>
      </div>
      <div className="p-section">
        <div className="p-title">{KINDS[kind] ?? kind} <span className="count">{docs.length}</span></div>
        {error && <div className="p-empty">{error}</div>}
        {docs.length === 0 && !error && (
          <div className="p-empty">还没有{KINDS[kind] ?? kind}。点「新建」建一份草稿。</div>
        )}
        {docs.map(d => (
          <div key={d.id} className="p-row click" title={d.slug}
            onClick={() => patch({doc: d.id})}>
            <span className="grow">{d.title}</span>
            <span className="sub">{d.activeRevisionId ? '已应用' : '草稿'}</span>
          </div>
        ))}
        <Button icon="plus" onClick={() => setCreating(true)}>新建</Button>
      </div>

      {creating && (
        <Prompt title={`新建${KINDS[kind] ?? kind}`} okLabel="创建"
          placeholder="标题，如：铁锭 → 铁块"
          onOk={title => {
            /* slug 由标题生成：后端要求 kind+slug 在包内唯一，且不许为空。
               中文标题转不出有意义 slug，用时间戳兜底，标题才是给人看的那个。 */
            const slug = `${kind}-${Date.now().toString(36)}`;
            void createContent(packId, {kind, slug, title}).then(load).catch(e => setError(String(e)));
          }}
          onClose={() => setCreating(false)}/>
      )}
    </div>
  );
}
