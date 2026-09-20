/* Emits the same save payload QuestEditorPage builds: edges authority → prerequisites sync. */
import {draftWithSyncedPrerequisites} from '../src/features/quest/questGraph.ts';

const draft = {
  chapters: [{id: 'ch1', title: '开始', description: '', coverColor: '#C9783B', position: 0}],
  nodes: [
    {id: 'A', chapterId: 'ch1', title: '任务 A', description: '', icon: '', x: 48, y: 48, prerequisites: [], rewards: [], modRefs: [], position: 0},
    {id: 'B', chapterId: 'ch1', title: '任务 B', description: '', icon: '', x: 248, y: 48, prerequisites: [], rewards: [], modRefs: [], position: 1},
  ],
  edges: [{id: 'e_A__B', fromNodeId: 'A', toNodeId: 'B'}],
};

const synced = draftWithSyncedPrerequisites(draft);
const bPrereq = (synced.nodes.find(n => n.id === 'B')?.prerequisites ?? []) as unknown[];
if (JSON.stringify(bPrereq) !== JSON.stringify(['A'])) {
  console.error('sync failed', bPrereq);
  process.exit(1);
}
process.stdout.write(JSON.stringify(synced));
