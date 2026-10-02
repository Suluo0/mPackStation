import {StubSettings} from '../stubs/StubSettings';

/* /settings（§6.6）：系统信息 + 目录配置 + 令牌状态。令牌只显示已配置/未配置。 */
export function SettingsPage() {
  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
      <div className="cp-panel">
        <div className="rr-title">系统自检与模组平台</div>
        <StubSettings/>
      </div>
      <div className="cp-panel">
        <div className="rr-title">游戏目录</div>
        <div className="rr-line">安装记录见交付页「③ 安装」段。</div>
      </div>
    </div>
  );
}
