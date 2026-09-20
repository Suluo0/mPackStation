import {useCallback, useEffect, useState} from 'react';
import {App} from 'antd';
import {ApiError} from '../api/http';
import {
  applyContent, applyQuest, getQuest, listContent, saveQuestDraft, validateContent, validateQuest,
  type ContentDocument, type QuestBook,
} from '../api/content';

/* 修订式编辑器的共用动作:校验/应用,412 revision_conflict 统一提示。 */
function useRevisionedActions(conflictText: string) {
  const {message} = App.useApp();
  const run = (fn: () => Promise<unknown>, ok: string) => {
    void fn().then(() => message.success(ok)).catch(e => {
      if (e instanceof ApiError && e.code === 'revision_conflict') { message.error(conflictText); return; }
      message.error(e instanceof Error ? e.message : String(e));
    });
  };
  return {run};
}

/* M1：edges 为图权威；坐标用画布网格值，避免全 0 被当成未定位。
   P0 FTB：book/节点扩展字段一并写入默认草稿。 */
export const defaultQuestDraft = {
  book: {title: '整合包任务书', icon: 'minecraft:book', progressionMode: 'flexible'},
  chapters: [{id: 'ch1', title: '开始', description: '', coverColor: '#C9783B', icon: 'minecraft:book', position: 0}],
  nodes: [{
    id: 'n1', chapterId: 'ch1', title: '入门', subtitle: '基础准备', description: '完成基础准备', icon: 'minecraft:wooden_pickaxe',
    x: 48, y: 48, shape: 'circle', size: 1, optional: false, invisible: false,
    dependencyRequirement: 'all_completed', minRequiredDependencies: 0,
    prerequisites: [] as unknown[], tasks: [{id: 'n1-t1', type: 'item', itemId: 'minecraft:wooden_pickaxe', count: 1}],
    rewards: [{kind: 'experience', experience: 10}] as unknown[], modRefs: [] as unknown[], position: 0,
  }],
  edges: [] as {id: string; fromNodeId: string; toNodeId: string}[],
};

/* 内容编辑场景:拉首个文档 + 校验/应用。 */
export function useContentEditor(packId: string) {
  const [doc, setDoc] = useState<ContentDocument | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!packId) return;
    void listContent(packId).then(v => setDoc(v[0] ?? null)).catch(e => setError(String(e)));
  }, [packId]);
  const {run} = useRevisionedActions('内容已被修改,请刷新后重试');
  return {
    doc, error,
    validate: () => doc && run(() => validateContent(packId, doc.id), '校验通过'),
    apply: () => doc && run(() => applyContent(packId, doc.id), '已应用'),
  };
}

const isMissingQuest = (e: unknown) => {
  if (e instanceof ApiError) {
    return e.status === 404
      || e.code === 'not_found'
      || e.code === 'pack_not_found'
      || e.code === 'quest_not_found'
      || /not found/i.test(e.message);
  }
  // zod 契约失败也视为“尚无可用任务书”，走自举而不是整页报错
  return e instanceof Error && /接口数据结构不符合约定/.test(e.message);
};

function emptyQuestBook(packId: string): QuestBook {
  return {
    id: `local-quest-${packId}`,
    packId,
    activeRevisionId: null,
    revision: {
      id: `local-rev-${packId}`,
      questBookId: `local-quest-${packId}`,
      state: 'draft',
      revision: 0,
      createdAt: new Date().toISOString(),
      draft: defaultQuestDraft,
    },
  };
}

/* 任务书:缺失/契约不完整时自动用默认草稿自举,避免点进页面直接报错。 */
export function useQuestBook(packId: string | undefined) {
  const {message} = App.useApp();
  const [book, setBook] = useState<QuestBook | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const {run} = useRevisionedActions('任务书已被修改,请刷新后重试');

  const reload = useCallback(async () => {
    if (!packId) return;
    setLoading(true);
    setError('');
    try {
      const b = await getQuest(packId);
      setBook(b);
    } catch (e) {
      if (!isMissingQuest(e)) {
        setError(e instanceof Error ? e.message : String(e));
        setBook(null);
        return;
      }
      try {
        await saveQuestDraft(packId, 0, defaultQuestDraft);
        const b = await getQuest(packId);
        setBook(b);
        message.success('已为该包创建初始任务书草稿');
      } catch (e2) {
        // 自举失败时先展示可编辑的本地草稿，不把用户堵在报错页
        if (isMissingQuest(e2) || e2 instanceof Error) {
          setBook(emptyQuestBook(packId));
          setError('');
        } else {
          setError(e2 instanceof Error ? e2.message : String(e2));
          setBook(null);
        }
      }
    } finally {
      setLoading(false);
    }
  }, [packId, message]);

  useEffect(() => { void reload(); }, [reload]);

  const saveDraft = useCallback(async (draft: unknown, ifMatch: number) => {
    if (!packId) return;
    setBusy(true);
    try {
      await saveQuestDraft(packId, ifMatch, draft);
      await reload();
      message.success('任务书草稿已保存');
    } catch (e) {
      if (e instanceof ApiError && e.code === 'revision_conflict') {
        message.error('任务书已被修改,请刷新后重试');
        return;
      }
      message.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [packId, message, reload]);

  return {
    book,
    error,
    loading,
    busy,
    reload,
    saveDraft,
    validate: () => run(() => validateQuest(packId!), '校验通过'),
    apply: () => run(() => applyQuest(packId!), '已应用'),
  };
}
