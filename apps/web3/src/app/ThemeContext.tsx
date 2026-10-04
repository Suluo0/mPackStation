import {createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useState, type ReactNode} from 'react';
import {ConfigProvider, theme as antdTheme} from 'antd';
import zhCN from 'antd/locale/zh_CN';
import {applyTheme, readStoredTheme, tokenValue, type ThemeName} from './theme';

/* 主题的唯一持有者。
   为什么要有这么一层：主题不只是「切个 CSS 变量」——antd 那套控件不吃 CSS 变量，
   它得在 JS 侧拿到色值。以前 main.tsx 在渲染前取一次，之后切主题 antd 就不会跟着变。
   这里把「当前主题」做成可订阅的 state，ConfigProvider 才会重新下发算法与令牌。

   色值本身仍然只在 styles/themes.css 里定义一份；这里是去读，不是去写。 */

type ThemeCtx = {
  theme: ThemeName;
  /** 传 null = 跟随系统 */
  setTheme: (name: ThemeName | null) => void;
  /** 用户有没有显式选过（null = 跟随系统） */
  preference: ThemeName | null;
};

const Ctx = createContext<ThemeCtx>({theme: 'light', setTheme: () => {}, preference: null});

export function useTheme(): ThemeCtx {
  return useContext(Ctx);
}

function systemTheme(): ThemeName {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/** antd 的令牌。在 data-theme 已经落定之后读，getComputedStyle 会强制重算，拿到的是新值。 */
function readAntdTokens() {
  const px = (name: string, fallback: number) => Number.parseFloat(tokenValue(name)) || fallback;
  return {
    colorPrimary: tokenValue('--mc-primary') || '#c9783b',
    colorError: tokenValue('--mc-fail') || '#e5484d',
    colorSuccess: tokenValue('--mc-success') || '#16a34a',
    colorWarning: tokenValue('--mc-orange') || '#ea8600',
    colorText: tokenValue('--mc-text') || '#252522',
    colorTextSecondary: tokenValue('--mc-text-2') || '#5e5a52',
    colorBgContainer: tokenValue('--mc-fill') || '#fffdf8',
    colorBgElevated: tokenValue('--mc-fill') || '#fffdf8',
    colorBorder: tokenValue('--mc-line') || '#e4ded2',
    borderRadius: px('--mc-radius', 4),
    fontSize: px('--mc-font-size', 13),
  };
}

export function ThemeProvider({children}: {children: ReactNode}) {
  const [preference, setPreference] = useState<ThemeName | null>(readStoredTheme);
  const [theme, setTheme] = useState<ThemeName>(() => preference ?? systemTheme());
  const [tokens, setTokens] = useState(readAntdTokens);

  /* 落 data-theme + 重读 antd 令牌，两件事必须在同一个 layout effect 里做完：
     分开做的话 antd 会拿上一帧的色值，切换时慢一拍。 */
  useLayoutEffect(() => {
    const resolved = applyTheme(preference);
    setTheme(resolved);
    setTokens(readAntdTokens());
  }, [preference]);

  /* 跟随系统：只有没显式选过才响应系统切换。 */
  useEffect(() => {
    if (preference !== null) return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => setTheme(applyTheme(null));
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, [preference]);

  const setThemePref = useCallback((name: ThemeName | null) => setPreference(name), []);
  const value = useMemo(() => ({theme, setTheme: setThemePref, preference}), [theme, setThemePref, preference]);

  return (
    <Ctx.Provider value={value}>
      <ConfigProvider
        locale={zhCN}
        theme={{
          algorithm: theme === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
          token: tokens,
        }}
      >
        {children}
      </ConfigProvider>
    </Ctx.Provider>
  );
}
