import {Outlet} from 'react-router-dom';
import {PackSummaryProvider} from './PackSummaryContext';
import {CatalogProvider} from './CatalogContext';
import {OnboardingProvider} from './OnboardingContext';
import {TopBar} from './TopBar';
import {RightRail} from './RightRail';
import {CommandPalette} from './CommandPalette';
import {OnboardingChecklist} from './OnboardingChecklist';
import {useHotkeys} from './useHotkeys';
import './shell.css';

/* 全局 Provider 都挂在这里：包汇总 + 内容目录（都依赖 useParams().id，互不依赖）+
   迎新（与包无关，挂最外层）。轮询在这两层，切态切页都不丢进度。 */
export function AppShell() {
  const [paletteOpen, openPalette] = useHotkeys();
  return (
    <OnboardingProvider>
      <PackSummaryProvider>
        <CatalogProvider>
          <div className="shell">
            <TopBar onOpenPalette={() => openPalette(true)}/>
            <div className="shell-body">
              <main className="shell-main"><Outlet/></main>
              <RightRail onOpenPalette={() => openPalette(true)}/>
            </div>
            <OnboardingChecklist/>
            <CommandPalette open={paletteOpen} onOpenChange={openPalette}/>
          </div>
        </CatalogProvider>
      </PackSummaryProvider>
    </OnboardingProvider>
  );
}
