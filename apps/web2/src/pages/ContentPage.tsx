import {useSearchParams} from 'react-router-dom';
import {SourceRail} from './content/SourceRail';
import {ModeTabs} from './content/ModeTabs';
import {CONTENT_MODES, type ContentMode} from '../app/nav';
import {StubItemIndex} from '../stubs/StubItemIndex';
import {StubRecipeGraph} from '../stubs/StubRecipeGraph';
import {StubContentEdit} from '../stubs/StubContentEdit';
import {StubQuest} from '../stubs/StubQuest';

/* 四态一个焦点：mode 只从这里读一次，四个态组件都是零 props，
   焦点由它们自己用 useFocus() 读 —— 这样第三步替换叶子时容器一行都不用改。 */
export function ContentPage() {
  const [params] = useSearchParams();
  const mode = (params.get('mode') ?? 'index') as ContentMode;
  const known = CONTENT_MODES.some(m => m.mode === mode) ? mode : 'index';
  return (
    <div className="content-page">
      <SourceRail mode={known}/>
      <section className="cp-canvas">
        <ModeTabs value={known}/>
        {known === 'index' && <StubItemIndex/>}
        {known === 'graph' && <StubRecipeGraph/>}
        {known === 'edit' && <StubContentEdit/>}
        {known === 'quest' && <StubQuest/>}
      </section>
    </div>
  );
}
