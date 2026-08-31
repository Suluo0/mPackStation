import {z} from 'zod';
import {get, put} from './http';

/* 迎新域:上手清单状态。prismAccount 由后端检测自动打勾,前端写它会 422。 */

export const onboardingSchema = z.object({
  steps: z.object({
    curseforgeKey: z.boolean(),
    firstPack: z.boolean(),
    firstMod: z.boolean(),
    prismAccount: z.boolean(),
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
