import {useEffect, useState} from 'react';
import {getPack, type Pack} from '../api/packs';

/* 单个整合包:页头上下文、工作台、模组页都要它。
   id 为空时不发请求,pack 保持 null。 */
export function usePack(id: string | undefined) {
  const [pack, setPack] = useState<Pack | null>(null);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!id) return;
    void getPack(id).then(setPack).catch(() => setPack(null));
  }, [id]);
  return {pack, error, setError};
}
