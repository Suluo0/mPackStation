import {useState} from 'react';
import {PackSummaryProvider} from './PackSummaryContext';
import {CatalogProvider} from './CatalogContext';
import {OnboardingProvider} from './OnboardingContext';
import {TopBar} from './TopBar';
import {IconRail} from './IconRail';
import {ToolPanel} from '../panels/ToolPanel';
import {EditorArea} from '../editor/EditorArea';
import {BottomDock} from '../dock/BottomDock';
import {StatusBar} from './StatusBar';
import {SettingsModal} from './SettingsModal';
import {OnboardingChecklist} from './OnboardingChecklist';
import {CommandPalette} from './CommandPalette';
import {useHotkeys} from './useHotkeys';
import {useUrlState} from './url';

/* 单页三区（V3 §1）：顶栏 / 图标轨+工具面板 / 编辑区，外加可停靠底部与状态条。
   全局 Provider 挂这里；?settings=1 是设置弹窗的深链（关闭时清参数）。 */
export function AppFrame() {
  const [paletteOpen, openPalette] = useHotkeys();
  const {settings} = useUrlState();
  const [panelW, setPanelW] = useState(() => {
    const v = Number(localStorage.getItem('web3.panelW'));
    return v >= 260 && v <= 480 ? v : 320;
  });

  return (
    <OnboardingProvider>
      <PackSummaryProvider>
        <CatalogProvider>
          <div className="frame">
            <TopBar onOpenPalette={() => openPalette(true)}/>
            <div className="frame-body">
              <IconRail/>
              <ToolPanel width={panelW} onResize={w => {
                const v = Math.min(480, Math.max(260, w));
                setPanelW(v);
                localStorage.setItem('web3.panelW', String(v));
              }}/>
              <EditorArea/>
            </div>
            <BottomDock/>
            <StatusBar onOpenPalette={() => openPalette(true)}/>
            <OnboardingChecklist/>
            <CommandPalette open={paletteOpen} onOpenChange={openPalette}/>
            <SettingsModal open={settings}/>
          </div>
        </CatalogProvider>
      </PackSummaryProvider>
    </OnboardingProvider>
  );
}
