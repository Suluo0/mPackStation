import {z} from 'zod';
import {get, put} from './http';

/* 迎新域:上手清单状态。
   离线启动优先:自研 mPackLauncher 不要求正版。
   prismAccount 兼容旧契约且恒为 true（废弃）；真实待办是 launcherReady。 */

export const onboardingSchema = z.object({
  steps: z.object({
    curseforgeKey: z.boolean(),
    firstPack: z.boolean(),
    firstMod: z.boolean(),
    prismAccount: z.boolean().default(true),
    launcherReady: z.boolean().default(false),
  }),
});
export type Onboarding = z.infer<typeof onboardingSchema>;

export function fetchOnboarding(): Promise<Onboarding> {
  return get('/api/onboarding', onboardingSchema);
}

/* 步骤打勾:后端接受 {"steps": {"<key>": true}},回填最新 onboarding 状态。 */
export function acknowledgeOnboarding(steps: Record<string, boolean>): Promise<Onboarding> {
  return put('/api/onboarding', {steps}, onboardingSchema);
}
