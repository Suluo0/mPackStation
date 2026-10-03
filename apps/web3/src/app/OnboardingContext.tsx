import {createContext, useCallback, useContext, useEffect, useState, type ReactNode} from 'react';
import {acknowledgeOnboarding, fetchOnboarding, type Onboarding} from '../api/onboarding';

type OnboardingState = {
  steps: Onboarding['steps'] | null;
  loading: boolean;
  error: string | null;
  ack: (key: keyof Onboarding['steps']) => Promise<void>;
  reload: () => Promise<void>;
};

const Ctx = createContext<OnboardingState | null>(null);

export function OnboardingProvider({children}: {children: ReactNode}) {
  const [steps, setSteps] = useState<Onboarding['steps'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const v = await fetchOnboarding();
      setSteps(v.steps);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void reload(); }, [reload]);

  /* 用后端回填的全量 steps 覆盖本地，不做乐观更新：
     后端可能同时推进别的键（例如建包时自动勾 firstPack），乐观写会把它盖掉。 */
  const ack = useCallback(async (key: keyof Onboarding['steps']) => {
    try {
      const v = await acknowledgeOnboarding({[key]: true});
      setSteps(v.steps);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  return <Ctx.Provider value={{steps, loading, error, ack, reload}}>{children}</Ctx.Provider>;
}

export function useOnboarding(): OnboardingState {
  const v = useContext(Ctx);
  if (!v) throw new Error('useOnboarding 必须在 OnboardingProvider 之内');
  return v;
}

/* UI 四步 ↔ 后端 steps 键。契约里还有一个恒为 true 的废弃键（见 api/onboarding.ts 注释），
   它不进清单。 */
export const CHECKLIST = [
  {key: 'firstPack', label: '新建 / 导入整合包'},
  {key: 'firstMod', label: '添加第一个模组'},
  {key: 'curseforgeKey', label: '配置 CurseForge Key'},
  {key: 'launcherReady', label: '启动器就绪并起窗'},
] as const satisfies readonly {key: keyof Onboarding['steps']; label: string}[];
