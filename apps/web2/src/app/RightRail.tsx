import {useState} from 'react';
import {useFocus} from './useFocus';
import {usePackSummary} from './PackSummaryContext';
import {StubFocusInspector} from '../stubs/StubFocusInspector';

const FOLD_KEY = 'web2.rail.collapsed';

/* 右栏两段（设计文档 §3 右栏两段表）。数据全部来自 usePackSummary()，
   本组件不许自己发请求（§8 禁止清单）。 */
export function RightRail({onOpenPalette}: {onOpenPalette: () => void}) {
  const {packId, pack, health, score, alertCount, loading, error} = usePackSummary();
  const [focus] = useFocus();
  const [folded, setFolded] = useState(() => localStorage.getItem(FOLD_KEY) === '1');

  if (!packId) return null;

  const toggleFold = () => {
    const next = !folded;
    setFolded(next);
    localStorage.setItem(FOLD_KEY, next ? '1' : '0');
  };

  const loaderLabel = pack ? pack.loader.charAt(0).toUpperCase() + pack.loader.slice(1) : '';

  return (
    <aside className="right-rail">
      <section className="rr-section">
        <div className="rr-head" onClick={toggleFold} role="button">
          <span className="rr-title">包状况</span>
          {folded && <span className="rr-line">健康 {score} · 告警 {alertCount}</span>}
          <button type="button" className="rr-fold" aria-label={folded ? '展开包状况' : '折叠包状况'}>
            {folded ? '▸' : '▾'}
          </button>
        </div>
        {!folded && (
          <div className="rr-body">
            <div className="rr-line">健康 {score} · 告警 {alertCount}</div>
            {pack && (
              <div className="rr-line">
                MC {pack.mcVersion} · {loaderLabel} · v{pack.packVersion} · {pack.modCount.total} 模组
              </div>
            )}
            {health && !health.healthy && <div className="rr-line">待处理：错误 {health.pendingErrors} · 警告 {health.pendingWarnings}</div>}
            {loading && <div className="rr-line">载入中…</div>}
            {error && <div className="rr-line">{error}</div>}
          </div>
        )}
      </section>

      <section className="rr-section rr-focus">
        <div className="rr-title">当前焦点</div>
        {focus
          ? <StubFocusInspector/>
          : (
            <div className="rr-empty">
              <span>还没有焦点。</span>
              {/* 空态必须指向下一个动作（设计文档 §5-9） */}
              <button type="button" className="tb-btn" onClick={onOpenPalette}>按 ⌘K 找一个物品</button>
            </div>
          )}
      </section>
    </aside>
  );
}
