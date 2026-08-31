import {useCallback, useEffect, useState} from 'react';
import {App} from 'antd';
import {listConflicts, listLocks, resolvePack, type Conflict, type Lock} from '../api/mods';

/* 依赖与冲突场景:加载冲突+锁列表,手动触发重新解析。 */
export function useDependencies(packId: string | undefined) {
  const {message} = App.useApp();
  const [conflicts, setConflicts] = useState<Conflict[]>([]);
  const [locks, setLocks] = useState<Lock[]>([]);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!packId) return;
    void Promise.all([listConflicts(packId), listLocks(packId)])
      .then(([c, l]) => { setConflicts(c); setLocks(l); })
      .catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId]);

  const resolve = useCallback(() => {
    if (!packId) return;
    void resolvePack(packId)
      .then(() => message.success('依赖已重新解析'))
      .catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, [packId, message]);

  return {conflicts, locks, error, resolve};
}
