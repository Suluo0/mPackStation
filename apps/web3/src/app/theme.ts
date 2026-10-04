/* 主题切换。唯一职责：在 <html> 上落 data-theme，并把它持久化。
   颜色值本身在 styles/themes.css，这里不存任何色值 —— 换配色改 CSS，不改这里。 */

export type ThemeName = 'light' | 'dark';

const KEY = 'mpack.theme';

function systemTheme(): ThemeName {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function readStoredTheme(): ThemeName | null {
  const v = localStorage.getItem(KEY);
  return v === 'dark' || v === 'light' ? v : null;
}

/** 落 data-theme 并持久化。传 null = 跟随系统（同时清掉存储值）。 */
export function applyTheme(name: ThemeName | null): ThemeName {
  const resolved = name ?? systemTheme();
  document.documentElement.dataset.theme = resolved;
  if (name) localStorage.setItem(KEY, name);
  else localStorage.removeItem(KEY);
  return resolved;
}

/** 首帧前调用，避免先渲染 light 再跳变。 */
export function initTheme(): ThemeName {
  return applyTheme(readStoredTheme());
}

/** 读一个设计令牌的当前值（主题切换后就是新值）。用于把变量喂给不吃 CSS 的库（如 antd）。 */
export function tokenValue(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}
