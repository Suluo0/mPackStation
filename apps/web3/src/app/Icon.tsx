/* 极简线性图标（16px 栅格，stroke 1.5）。不引图标库：六个图标手写足够，
   风格与 IDE 原生 chrome 一致。 */
const PATHS: Record<string, string> = {
  focus: 'M8 1.5v3M8 11.5v3M1.5 8h3M11.5 8h3M8 5.5A2.5 2.5 0 1 0 8 10.5 2.5 2.5 0 0 0 8 5.5z',
  sources: 'M8 2 14 5 8 8 2 5zM2 8.5 8 11.5 14 8.5M2 11.5 8 14.5 14 11.5',
  problems: 'M8 2 14.5 13.5H1.5zM8 6.5v3.2M8 11.6v.9',
  build: 'M4.5 2.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zM4.5 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zM11.5 2.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3zM4.5 5.5v5M11.5 5.5v1c0 1.7-1.3 3-3 3H6',
  run: 'M4.5 3.2v9.6L12.5 8z',
  search: 'M7 2.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9zM10.5 10.5 13.5 13.5',
  plus: 'M8 3v10M3 8h10',
  filter: 'M2.5 3.5h11L9.5 8.3v4.4l-3 1.6V8.3z',
  sortdown: 'M5 2.5v7.6M5 13.5 2.2 10.3h5.6z',
  sortup: 'M11 13.5V5.9M11 2.5 8.2 5.7h5.6z',
  settings: 'M8 5.8A2.2 2.2 0 1 0 8 10.2 2.2 2.2 0 0 0 8 5.8zM8 1.8v1.7M8 12.5v1.7M2.6 4.9l1.5.9M11.9 10.2l1.5.9M2.6 11.1l1.5-.9M11.9 5.8l1.5-.9',
};

export function Icon({name, size = 16}: {name: keyof typeof PATHS | string; size?: number}) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" aria-hidden
      style={{flex: 'none'}}>
      <path d={PATHS[name] ?? ''} stroke="currentColor" strokeWidth="1.5"
        strokeLinecap="round" strokeLinejoin="round"/>
    </svg>
  );
}
