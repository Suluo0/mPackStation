import {useCallback, useEffect, useState} from 'react';
import {App} from 'antd';
import {ApiError} from '../api/http';
import {
  applyContent, applyQuest, getQuest, listContent, saveQuestDraft,
  validateContent, validateQuest,
  type ContentDocument, type QuestBook, type QuestValidation,
} from '../api/content';

function formatIssues(issues: QuestValidation['issues']): string {
  if (!issues?.length) return '（无问题）';
  return issues.slice(0, 12).map(i => {
    const sev = i.severity || 'info';
    const path = i.path ? ` @${i.path}` : '';
    return `· [${sev}] ${i.code || 'issue'}${path}: ${i.message || ''}`;
  }).join('\n');
}

function useRevisionedActions(conflictText: string) {
  const {message} = App.useApp();
  const run = (fn: () => Promise<unknown>, ok: string) => {
    void fn().then(() => message.success(ok)).catch(e => {
      if (e instanceof ApiError && (e.code === 'revision_conflict' || e.status === 409 || e.status === 412)) {
        message.error(e.message || conflictText);
        return;
      }
      message.error(e instanceof Error ? e.message : String(e));
    });
  };
  return {run};
}

export const defaultQuestDraft = {
  book: {title: '整合包任务书', icon: 'minecraft:book', progressionMode: 'flexible'},
  chapters: [{id: 'ch1', title: '开始', description: '', coverColor: '#C9783B', icon: 'minecraft:book', position: 0}],
  nodes: [{
    id: 'n1', chapterId: 'ch1', title: '入门', subtitle: '基础准备', description: '完成基础准备', icon: 'minecraft:wooden_pickaxe',
    x: 48, y: 48, shape: 'circle', size: 1, optional: false, invisible: false,
    dependencyRequirement: 'all_completed', minRequiredDependencies: 0,
    prerequisites: [] as unknown[],
    tasks: [{id: 'n1-t1', type: 'item', itemId: 'minecraft:wooden_pickaxe', count: 1}],
    rewards: [{kind: 'experience', experience: 10}] as unknown[],
    modRefs: [] as unknown[],
    position: 0,
  }],
  edges: [] as {id: string; fromNodeId: string; toNodeId: string}[],
};

export function useContentEditor(packId: string) {
  const [doc, setDoc] = useState<ContentDocument | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
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

export type QuestActionResult = {
  ok: boolean;
  status?: string;
  issues?: QuestValidation['issues'];
  revisionId?: string | null;
};

export type QuestLastAction = {type: 'validate' | 'apply' | 'save'; status: string; text: string};

export function useQuestBook(packId: string | undefined) {
  const {message, modal} = App.useApp();
  const [book, setBook] = useState<QuestBook | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [lastAction, setLastAction] = useState<QuestLastAction | null>(null);

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
      } catch {
        setBook(emptyQuestBook(packId));
        setError('');
      }
    } finally {
      setLoading(false);
    }
  }, [packId, message]);

  useEffect(() => { void reload(); }, [reload]);

  const saveDraft = useCallback(async (draft: unknown, ifMatch: number): Promise<boolean> => {
    if (!packId) return false;
    setBusy(true);
    try {
      await saveQuestDraft(packId, ifMatch, draft);
      await reload();
      setLastAction({type: 'save', status: 'ok', text: '草稿已保存'});
      message.success('任务书草稿已保存');
      return true;
    } catch (e) {
      if (e instanceof ApiError && (e.code === 'revision_conflict' || e.status === 409 || e.status === 412)) {
        setLastAction({type: 'save', status: 'conflict', text: e.message || '修订冲突'});
        message.error('任务书已被修改,请刷新后重试');
        return false;
      }
      const msg = e instanceof Error ? e.message : String(e);
      setLastAction({type: 'save', status: 'error', text: msg});
      message.error(msg);
      return false;
    } finally {
      setBusy(false);
    }
  }, [packId, message, reload]);

  const showIssues = useCallback((title: string, result: QuestValidation) => {
    const issues = result.issues ?? [];
    const failed = result.status === 'failed';
    const warn = result.status === 'warning' || issues.length > 0;
    if (failed) {
      setLastAction({type: 'validate', status: 'failed', text: `校验未通过（${issues.length}）`});
    } else if (warn) {
      setLastAction({type: 'validate', status: result.status || 'warning', text: `校验警告（${issues.length}）`});
    } else {
      setLastAction({type: 'validate', status: 'passed', text: '校验通过'});
    }
    if (!issues.length) {
      message.success('校验通过');
      return;
    }
    modal.info({
      title,
      width: 560,
      content: formatIssues(issues),
    });
  }, [message, modal]);

  const validate = useCallback(async (): Promise<QuestActionResult | null> => {
    if (!packId) return null;
    setBusy(true);
    try {
      const result = await validateQuest(packId);
      showIssues(`校验结果 · ${result.status}`, result);
      return {ok: result.status !== 'failed', status: result.status, issues: result.issues, revisionId: result.revisionId};
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      setLastAction({type: 'validate', status: 'error', text: msg});
      message.error(msg);
      return {ok: false};
    } finally {
      setBusy(false);
    }
  }, [packId, message, showIssues]);

  const apply = useCallback(async (): Promise<QuestActionResult | null> => {
    if (!packId) return null;
    setBusy(true);
    try {
      const result = await applyQuest(packId);
      await reload();
      const st = (result as {status?: string}).status || '';
      if (st === 'applied' || st === 'ok' || st === 'success') {
        setLastAction({type: 'apply', status: 'applied', text: '已应用'});
        message.success('已应用');
        return {ok: true, status: st, revisionId: result.revisionId};
      }
      setLastAction({type: 'apply', status: st || 'unknown', text: `应用返回 ${st || '空状态'}`});
      message.warning(`应用请求完成，但状态为「${st || '未知'}」，已刷新页面数据`);
      return {ok: false, status: st, revisionId: result.revisionId};
    } catch (e) {
      await reload();
      if (e instanceof ApiError && (e.status === 409 || e.code === 'quest_apply_conflict' || e.code === 'revision_conflict')) {
        setLastAction({type: 'apply', status: 'conflict', text: e.message || '应用冲突'});
        message.error(e.message || '当前修订无法应用（可能已是 applied，请先保存新草稿再应用）');
        return {ok: false, status: 'conflict'};
      }
      if (e instanceof ApiError && (e.status === 422 || e.code === 'validation_failed' || e.code === 'quest_validation_failed')) {
        setLastAction({type: 'apply', status: 'validation_failed', text: '校验阻断，未应用'});
        message.error('存在阻断性校验问题，未应用。');
        try {
          const preview = await validateQuest(packId);
          showIssues('应用前校验未通过', preview);
        } catch { /* ignore */ }
        return {ok: false, status: 'validation_failed'};
      }
      const msg = e instanceof Error ? e.message : String(e);
      setLastAction({type: 'apply', status: 'error', text: msg});
      message.error(msg);
      return {ok: false};
    } finally {
      setBusy(false);
    }
  }, [packId, message, reload, showIssues]);

  return {
    book,
    error,
    loading,
    busy,
    lastAction,
    reload,
    saveDraft,
    validate,
    apply,
  };
}
