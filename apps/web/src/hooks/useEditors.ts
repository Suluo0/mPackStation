import {useEffect, useState} from 'react';
import {App} from 'antd';
import {ApiError} from '../api/http';
import {
  applyContent, applyQuest, getQuest, listContent, validateContent, validateQuest,
  type ContentDocument, type QuestBook,
} from '../api/content';

/* 修订式编辑器的共用动作:校验/应用,412 revision_conflict 统一提示。
   内容文档与任务书共用同一套乐观锁语义,只是文案不同。 */
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

/* 内容编辑场景:拉首个文档 + 校验/应用。 */
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

/* 任务书编辑场景:拉任务书 + 校验/应用。 */
export function useQuestBook(packId: string) {
  const [book, setBook] = useState<QuestBook | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    void getQuest(packId).then(setBook).catch(e => setError(String(e)));
  }, [packId]);
  const {run} = useRevisionedActions('任务书已被修改,请刷新后重试');
  return {
    book, error,
    validate: () => run(() => validateQuest(packId), '校验通过'),
    apply: () => run(() => applyQuest(packId), '已应用'),
  };
}
