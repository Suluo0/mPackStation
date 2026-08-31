import {useCallback, useEffect, useState} from 'react';
import {listPacks, type Pack} from '../api/packs';

/* 整合包列表:包列表页、外壳的"第一个包"定位、路由重定向共用。 */
export function usePacks() {
  const [packs, setPacks] = useState<Pack[]>([]);
  const [error, setError] = useState('');
  const reload = useCallback(() => {
    void listPacks().then(setPacks).catch(e => setError(e instanceof Error ? e.message : String(e)));
  }, []);
  useEffect(reload, [reload]);
  return {packs, error, reload};
}
