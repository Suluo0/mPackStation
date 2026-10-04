import {Icon} from './Icon';

/* 加载指示：全站唯一的 spinner，任何「正在忙」的位置都用它，不许各画一份。
   形状来自 lucide loader-pinwheel（外圈 + 三条等距弧），旋转动画在 ui.css 的
   .ui-spin —— 颜色走 --mc-primary，换主题自动跟随。

   为什么不用 antd 的 Spin：它是四个圆点的转法，和这里的口径不同；
   而且它默认吃 antd 的 primary 色，不跟 --mc-* 令牌，深色主题下会跑出一套蓝。 */
export function Spinner({size = 14, label, style}: {
  size?: number;
  label?: string;
  style?: React.CSSProperties;
}) {
  /* aria-hidden 的 svg 屏幕阅读器读不到，所以状态由外层 span 承担：
     role=status 让读屏软件在内容变化时播报一次，aria-label 说明在忙什么。 */
  return (
    <span className="ui-spin" role="status" aria-label={label ?? '加载中'} style={style}>
      <Icon name="spinner" size={size}/>
    </span>
  );
}
