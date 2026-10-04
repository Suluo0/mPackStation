/* 图标统一走 lucide-react（ISC，24 栅格纯描边 / 圆头圆角），调用点只认语义名。
   **不在这里手画任何 path** —— 手画的形状和库里的线宽/圆角/视觉重量对不齐，
   16px 下一眼就能看出是两套东西；需要新图标就在库里挑，挑不到就不加。

   为什么不是 @ant-design/icons：它在依赖里躺着（零引用），但那套是 1024 栅格、
   实心与描边混排、线宽写死在 path 里，和这里 16px 的线性风格不同路。 */
import {
  ArrowDownNarrowWide, ArrowUpNarrowWide, BookOpen, ChevronDown, ChevronLeft,
  ChevronRight, ChevronUp, Copy, Folder, FolderOpen, FolderPlus, Funnel, Hammer,
  Handbag, Layers, LoaderPinwheel, Pause, Play, Plus, RotateCw, Search, Settings,
  Square, Target, Trash2, TriangleAlert, Wrench, type LucideIcon,
} from 'lucide-react';

const ICONS: Record<string, LucideIcon> = {
  focus: Target,
  sources: Layers,
  problems: TriangleAlert,
  run: Play,
  play: Play,
  pause: Pause,
  stop: Square,
  refresh: RotateCw,
  wrench: Wrench,
  quest: BookOpen,
  /* 商店用「手提包」而不是 ShoppingCart/Store：这是模组包管理器的货架，
     不是电商收银台，包里没车也没价签。 */
  store: Handbag,
  folder: Folder,
  folderOpen: FolderOpen,
  folderPlus: FolderPlus,
  filter: Funnel,
  sortdown: ArrowDownNarrowWide,
  sortup: ArrowUpNarrowWide,
  search: Search,
  plus: Plus,
  trash: Trash2,
  /* 「Get Version」= 版本跟踪：分支分叉，左边两个点、右边一个点。
     这个形状 lucide 里没有等价物（git-branch 只有两个点、git-fork 是「上二下一」），
     所以走上面的 LEGACY 表，从 git 历史原样取回 —— **不重画、不旋转**。 */
  build: Hammer,
  settings: Settings,
  hammer: Hammer,
  copy: Copy,
  /* 加载指示：lucide 的 loader-pinwheel = 外圈 + 三条 120° 等距弧，
     转起来就是标准的 spinner。用它而不是自己写 CSS 圆环，理由和上面一样 ——
     自写的线宽/圆角/视觉重量和库里这套图标不是一路货。
     不用 antd 的 Spin：它是四个圆点的转法，且默认吃 antd 的 primary 色，
     不跟 --mc-* 主题令牌。 */
  spinner: LoaderPinwheel,
  /* 展开/收起、翻页、面包屑一律用库里的 chevron。
     以前这些位置写的是文本字符 '▸▾‹›' —— 它的字重和垂直位置由字体决定，
     和旁边的 SVG 描边对不齐，同一个「箭头」在不同字体下还不一样。 */
  caretRight: ChevronRight,
  caretDown: ChevronDown,
  caretLeft: ChevronLeft,
  caretUp: ChevronUp,
};

/* 历史遗留的 16 栅格手绘图标（**原样取自 git 历史，不是新画的**：
   `git show HEAD:apps/web3/src/app/Icon.tsx` 的 PATHS 表）。

   这些形状在 lucide 里没有等价物（git-branch 只有两个点，git-fork 是
   「上二下一」），改一个像素就等于换图标。所以这里不做迁移、不重画：
   渲染走自己的 16 栅格 + stroke 1.5，和 lucide 的 24 栅格各走各的。 */
const LEGACY: Record<string, string> = {
  /* Get Version = 版本跟踪：分支分叉，左边两个点（上 4.5,4 / 下 4.5,12）、
     右边一个点（11.5,4），右点的线向下弯进左竖线。这就是历史上
     build 按钮那个图标。 */
  releases: 'M4.5 2.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z'
    + 'M4.5 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z'
    + 'M11.5 2.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3z'
    + 'M4.5 5.5v5M11.5 5.5v1c0 1.7-1.3 3-3 3H6',
};

/* 图标渲染。名字不在表里就返回 null —— 画一个空 svg 只会得到一排
   「看着是空的」按钮，排查时还得回源码对名字；缺了就是缺了，一眼能看见。 */
export function Icon({name, size = 16}: {name: string; size?: number}) {
  const legacy = LEGACY[name];
  if (legacy) {
    return (
      <svg width={size} height={size} viewBox="0 0 16 16" fill="none" aria-hidden
        style={{flex: 'none', verticalAlign: '-0.125em'}}>
        <path d={legacy} stroke="currentColor" strokeWidth="1.5"
          strokeLinecap="round" strokeLinejoin="round"/>
      </svg>
    );
  }
  const C = ICONS[name];
  if (!C) return null;
  return <C size={size} strokeWidth={2} aria-hidden style={{flex: 'none', verticalAlign: '-0.125em'}}/>;
}
