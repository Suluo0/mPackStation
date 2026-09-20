import { useCallback, type PointerEvent as ReactPointerEvent } from 'react';
import './MpCanvas.css';

export type MpZoomBarProps = {
  k: number;
  min: number;
  max: number;
  onZoomBy: (factor: number) => void;
  onZoomTo: (k: number) => void;
  onReset: () => void;
  onFit: () => void;
  className?: string;
  testId?: string;
  /** 坞左侧自定义内容（如「工作台增强」标注） */
  leftSlot?: React.ReactNode;
  percentTestId?: string;
  btnTestIds?: {
    zoomIn?: string;
    zoomOut?: string;
    fit?: string;
    reset?: string;
    slider?: string;
  };
  showSlider?: boolean;
  ariaLabel?: string;
};

/**
 * 底部缩放坞：重置 / 适应 / − / 滑杆 / + / %
 * 本身 data-canvas-no-zoom + stopPropagation，防止 pointer/wheel 被画布抢走。
 */
export function MpZoomBar({
  k,
  min,
  max,
  onZoomBy,
  onZoomTo,
  onReset,
  onFit,
  className,
  testId = 'mp-zoom-bar',
  leftSlot,
  percentTestId = 'mp-zoom-percent',
  btnTestIds,
  showSlider = true,
  ariaLabel = '画布缩放坞',
}: MpZoomBarProps) {
  const pct = Math.round(k * 100);
  const stop = useCallback((e: ReactPointerEvent | React.MouseEvent) => {
    e.stopPropagation();
  }, []);

  const onSlider = (e: React.ChangeEvent<HTMLInputElement>) => {
    const next = Number(e.target.value) / 100;
    onZoomTo(next);
  };

  return (
    <div
      className={`mp-zoom-bar ${className ?? ''}`.trim()}
      data-canvas-no-zoom=""
      data-testid={testId}
      aria-label={ariaLabel}
      role="group"
      onPointerDown={stop}
      onPointerMove={stop}
      onPointerUp={stop}
      onWheel={e => e.stopPropagation()}
      onClick={stop}
      onDoubleClick={stop}
    >
      {leftSlot}
      <button
        type="button"
        data-testid={btnTestIds?.reset ?? 'mp-zoom-btn-reset'}
        aria-label="重置视图"
        title="重置（0）"
        onClick={onReset}
      >
        重置
      </button>
      <button
        type="button"
        data-testid={btnTestIds?.fit ?? 'mp-zoom-btn-fit'}
        aria-label="适应内容"
        title="适应内容（1）"
        onClick={onFit}
      >
        适应
      </button>
      <button
        type="button"
        data-testid={btnTestIds?.zoomOut ?? 'mp-zoom-btn-zoom-out'}
        aria-label="缩小"
        title="缩小（−）"
        onClick={() => onZoomBy(1 / 1.25)}
      >
        −
      </button>
      {showSlider && (
        <input
          className="mp-zoom-slider"
          type="range"
          min={Math.round(min * 100)}
          max={Math.round(max * 100)}
          step={1}
          value={pct}
          data-testid={btnTestIds?.slider ?? 'mp-zoom-slider'}
          aria-label={`缩放滑杆 ${Math.round(min * 100)}%–${Math.round(max * 100)}%`}
          onChange={onSlider}
        />
      )}
      <button
        type="button"
        data-testid={btnTestIds?.zoomIn ?? 'mp-zoom-btn-zoom-in'}
        aria-label="放大"
        title="放大（+）"
        onClick={() => onZoomBy(1.25)}
      >
        +
      </button>
      <span
        className="mp-zoom-percent"
        data-testid={percentTestId}
        aria-label={`缩放 ${pct}%`}
      >
        {pct}%
      </span>
    </div>
  );
}

export default MpZoomBar;
