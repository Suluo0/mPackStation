import './recipe.css';

/* 共享 JEI 风格合成器视图：3×3 网格 / 机器配方 / 特殊配方 / 弹药。
   原本内嵌在 pages/ModContentPage.tsx，现抽出供「合成器页」「魔改预览」「Inspector 配方卡」复用（DRY）。
   纯展示组件：靠 translateKey/getItemIcon 注入命名与图标，onSelect 回调把点击穿透交给上层（设焦点）。 */

/* 原版/特殊合成类型：运行时逻辑，无固定合成表。 */
export function isSpecialRecipeType(type: string): boolean {
  return type.startsWith('minecraft:crafting_special_') || type === 'minecraft:crafting_decorated_pot';
}

/* payload 是否属于特殊/动态配方（无 ingredients/result）。 */
export function isSpecialRecipePayload(payload: unknown): boolean {
  if (!payload || typeof payload !== 'object') return false;
  const p = payload as Record<string, unknown>;
  const type = String(p.type || '');
  if (type === 'ae2:matter_cannon') return false;
  if (isSpecialRecipeType(type)) return true;
  return type !== '' && p.ingredients === undefined && p.ingredient === undefined
    && p.result === undefined && p.output === undefined;
}

/* 机器配方类型标签（非 3×3 合成表） */
export const MACHINE_RECIPE_LABELS: Record<string, string> = {
  'minecraft:stonecutting': '切石机',
  'minecraft:smelting': '熔炉',
  'minecraft:blasting': '高炉',
  'minecraft:smoking': '烟熏炉',
  'minecraft:campfire_cooking': '营火',
  'minecraft:smithing_transform': '锻造台',
  'minecraft:smithing_trim': '锻造台·纹饰',
};

/** 处理设备的方块图标（叠在箭头上，替代文字胶囊） */
export const MACHINE_RECIPE_ICONS: Record<string, string> = {
  'minecraft:stonecutting': 'minecraft:stonecutter',
  'minecraft:smelting': 'minecraft:furnace',
  'minecraft:blasting': 'minecraft:blast_furnace',
  'minecraft:smoking': 'minecraft:smoker',
  'minecraft:campfire_cooking': 'minecraft:campfire',
  'minecraft:smithing_transform': 'minecraft:smithing_table',
  'minecraft:smithing_trim': 'minecraft:smithing_table',
};

export function SlotIcon({itemId, translateKey, getItemIcon, onSelect}: {
  itemId: string;
  translateKey: (k: string) => string;
  getItemIcon: (k: string) => string | null;
  onSelect: (id: string) => void;
}) {
  if (!itemId) {
    return <div className="recipe-slot empty"><span className="recipe-item-name">—</span></div>;
  }
  return (
    <div className="recipe-slot filled" onClick={() => onSelect(itemId)} title={itemId}>
      {getItemIcon(itemId) && <img className="recipe-icon" src={getItemIcon(itemId)!} alt=""/>}
      <span className="recipe-item-name">{translateKey(itemId)}</span>
    </div>
  );
}

export function ingredientId(ing: unknown): string {
  if (!ing) return '';
  if (typeof ing === 'string') return ing;
  const o = ing as {item?: string; id?: string; tag?: string};
  return o.item || o.id || (o.tag ? `#${o.tag}` : '');
}

export function resultId(res: unknown): {id: string; count: number} | null {
  if (!res) return null;
  if (typeof res === 'string') return {id: res, count: 1};
  const o = res as {id?: string; item?: string; count?: number};
  const id = o.id || o.item || '';
  if (!id) return null;
  return {id, count: Number(o.count || 1)};
}

export type ParsedRecipe = {
  grid: (string | null)[][];
  output: {id: string; count: number} | null;
  type: string;
  special: boolean;
  machine: string | null;
  machineLabel: string | null;
  machineInputs: string[];
  extra?: Record<string, string>;
};

/* 解析配方 payload：合成网格 或 机器配方（切石机/熔炉等）。 */
export function parseRecipeGrid(payload: unknown): ParsedRecipe {
  if (!payload || typeof payload !== 'object') {
    return {grid: [], output: null, type: 'unknown', special: false, machine: null, machineLabel: null, machineInputs: []};
  }
  const p = payload as Record<string, unknown>;
  const type = String(p.type || 'unknown');
  const result = resultId(p.result) ?? resultId(p.output);
  const special = isSpecialRecipePayload(payload);

  if (!special && type === 'minecraft:crafting_shaped' && Array.isArray(p.pattern) && p.key && typeof p.key === 'object') {
    const key = p.key as Record<string, {item?: string; id?: string; tag?: string}>;
    const grid: (string | null)[][] = [];
    for (let row = 0; row < 3; row++) {
      const gridRow: (string | null)[] = [];
      const patternRow = String((p.pattern as string[])[row] || '');
      for (let col = 0; col < 3; col++) {
        const char = patternRow[col] || ' ';
        if (char === ' ') { gridRow.push(null); continue; }
        const keyEntry = key[char];
        const itemId = keyEntry?.item || keyEntry?.id || (keyEntry?.tag ? `#${keyEntry.tag}` : '');
        gridRow.push(itemId || null);
      }
      grid.push(gridRow);
    }
    return {grid, output: result, type, special: false, machine: null, machineLabel: null, machineInputs: []};
  }

  if (!special && (type === 'minecraft:crafting_shapeless' || type === 'ae2:transform') && Array.isArray(p.ingredients)) {
    const grid: (string | null)[][] = [[null, null, null], [null, null, null], [null, null, null]];
    let idx = 0;
    for (const ing of p.ingredients as Array<{item?: string; id?: string; tag?: string}>) {
      const itemId = ing?.item || ing?.id || (ing?.tag ? `#${ing.tag}` : '');
      if (itemId && idx < 9) {
        grid[Math.floor(idx / 3)][idx % 3] = itemId;
        idx++;
      }
    }
    return {grid, output: result, type, special: false, machine: null, machineLabel: null, machineInputs: []};
  }

  // 切石机 / 熔炉类 / 锻造台
  const machineLabel = MACHINE_RECIPE_LABELS[type];
  if (!special && machineLabel) {
    const inputs: string[] = [];
    if (type === 'minecraft:smithing_transform') {
      for (const k of ['template', 'base', 'addition']) {
        const id = ingredientId(p[k]);
        if (id) inputs.push(id);
      }
    } else {
      const id = ingredientId(p.ingredient) || ingredientId(p.ingredient0);
      if (id) inputs.push(id);
    }
    const extra: Record<string, string> = {};
    if (typeof p.cookingtime === 'number') extra.cookTime = `${p.cookingtime} tick`;
    if (typeof p.experience === 'number') extra.experience = String(p.experience);
    return {grid: [], output: result, type, special: false, machine: type, machineLabel, machineInputs: inputs, extra};
  }

  return {grid: [], output: result, type, special, machine: null, machineLabel: null, machineInputs: []};
}

/* 配方解锁进度（advancement）不是合成配方：payload 有 parent/criteria，无 type。 */
export function isAdvancementPayload(payload: unknown): boolean {
  if (!payload || typeof payload !== 'object') return false;
  const p = payload as Record<string, unknown>;
  return (typeof p.criteria === 'object' && p.criteria !== null)
    || (typeof p.parent === 'string' && String(p.parent).includes('recipes/'))
    || (typeof p.rewards === 'object' && p.rewards !== null && typeof p.type !== 'string');
}

const SPECIAL_HINTS: {match: (t: string) => boolean; text: string}[] = [
  {match: t => t.includes('bookcloning'), text: '书与笔复制：将「书与笔」与「已写成的书」放入合成栏，可复制书的内容（每份副本消耗书与笔的墨水）。'},
  {match: t => t.includes('bannerduplicate') || t.includes('banner_duplicate'), text: '旗帜复制：任意 1 面旗帜 + 任意 1 个染料 → 2 面同款旗帜（含已印图案）。染料颜色不影响复制结果。'},
  {match: t => t.includes('armordye'), text: '盔甲染色：皮革盔甲 + 染料，可染成对应颜色（可多染料混色）。'},
  {match: t => t.includes('mapcloning'), text: '地图复制：已有地图 + 空地图 → 两份相同地图。'},
  {match: t => t.includes('mapextending'), text: '地图扩展：已有地图 + 纸 → 扩大地图比例尺。'},
  {match: t => t.includes('repairitem'), text: '物品修理：两件同类工具/武器在合成栏合并，按耐久计算修复结果。'},
  {match: t => t.includes('firework_rocket'), text: '烟花火箭：火药 + 纸（可选烟火之星）→ 烟花；火药数量影响飞行时长。'},
  {match: t => t.includes('tippedarrow'), text: '药箭：箭 + 滞留型药水 → 对应药水效果的药箭。'},
  {match: t => t.includes('shulkerboxcoloring'), text: '潜影盒染色：潜影盒 + 染料 → 同色潜影盒。'},
  {match: t => t.includes('suspiciousstew'), text: '迷之炖菜：碗 + 红蘑菇 + 棕蘑菇（+ 可选花）→ 迷之炖菜，效果随花变化。'},
];

/* JEI 风格配方合成网格视图: 3x3 输入 + 箭头 + 输出。special 显示运行时逻辑说明。 */
export function RecipeViewer({payload, translateKey, getItemIcon, onSelect, itemKind}: {
  payload: unknown;
  translateKey: (k: string) => string;
  getItemIcon: (k: string) => string | null;
  onSelect: (id: string) => void;
  itemKind?: string;
}) {
  const {grid, output, type, special, machine, machineLabel, machineInputs, extra} = parseRecipeGrid(payload);

  if (itemKind === 'advancement' || (type === 'unknown' && isAdvancementPayload(payload))) {
    return (
      <div className="recipe-viewer unsupported">
        这是<strong>配方解锁进度（advancement）</strong>，不是合成配方。
        原版会把 <code>data/&lt;ns&gt;/advancement/recipes/**</code> 用来解锁配方展示，真正的合成配方在 <code>recipe/</code> 目录。
      </div>
    );
  }

  // 机器配方：原料 → 箭头（叠处理设备图标）→ 产物
  if (machine && machineLabel) {
    const machineIconId = MACHINE_RECIPE_ICONS[machine] || '';
    const machineIcon = machineIconId ? getItemIcon(machineIconId) : null;
    return (
      <div className="recipe-viewer machine-recipe" data-machine={machine}>
        <div className="recipe-row" style={{alignItems: 'center'}}>
          {(machineInputs.length ? machineInputs : ['']).map((itemId, i) => (
            <SlotIcon key={i} itemId={itemId} translateKey={translateKey} getItemIcon={getItemIcon} onSelect={onSelect}/>
          ))}
        </div>
        <div className="recipe-process" title={machineLabel} data-testid="recipe-process-icon">
          {machineIcon ? (
            <img className="recipe-process-icon" src={machineIcon} alt={machineLabel}/>
          ) : (
            <span className="recipe-process-icon placeholder" aria-label={machineLabel}>
              {machineLabel.slice(0, 1)}
            </span>
          )}
          <div className="recipe-arrow-process" aria-hidden>→</div>
        </div>
        <div className="recipe-output">
          {output ? (
            <div className="recipe-slot filled output-slot" onClick={() => onSelect(output.id)} title={output.id}>
              {getItemIcon(output.id) && <img className="recipe-icon" src={getItemIcon(output.id)!} alt=""/>}
              <span className="recipe-item-name">{translateKey(output.id)}</span>
              {output.count > 1 && <span className="recipe-count">×{output.count}</span>}
            </div>
          ) : <div className="recipe-slot empty"><span className="recipe-item-name">无输出</span></div>}
        </div>
        {extra && Object.keys(extra).length > 0 && (
          <div className="machine-recipe-extra">
            {Object.entries(extra).map(([k, v]) => <span key={k}>{k}: {v}</span>)}
          </div>
        )}
      </div>
    );
  }

  // ae2:matter_cannon 弹药属性, 不是合成配方
  if (type === 'ae2:matter_cannon' && payload && typeof payload === 'object') {
    const p = payload as Record<string, unknown>;
    const ammo = p.ammo as {item?: string; id?: string} | undefined;
    const ammoId = ammo?.item || ammo?.id || '';
    const weight = Number(p.weight || 0);
    return (
      <div className="recipe-viewer ammo-viewer">
        <div className="ammo-card">
          <div className="ammo-label">弹药</div>
          <div className="recipe-slot filled">
            {ammoId && getItemIcon(ammoId) && <img className="recipe-icon" src={getItemIcon(ammoId)!} alt=""/>}
            <span className="recipe-item-name">{ammoId ? translateKey(ammoId) : '未知'}</span>
          </div>
          <div className="ammo-weight">重量: <strong>{weight}</strong></div>
        </div>
      </div>
    );
  }

  if (special) {
    const hint = SPECIAL_HINTS.find(h => h.match(type));
    return (
      <div className="recipe-viewer special-recipe">
        <div className="special-recipe-note">特殊合成配方（运行时逻辑，无固定合成表）</div>
        <div className="special-recipe-type"><code>{type}</code></div>
        {hint && <div className="special-recipe-hint">{hint.text}</div>}
        {output && (
          <div className="special-recipe-output">
            可能产出：<code onClick={() => onSelect(output.id)} style={{cursor: 'pointer'}}>{translateKey(output.id)}</code>
          </div>
        )}
      </div>
    );
  }

  if (!grid.length) {
    return <div className="recipe-viewer unsupported">该配方类型暂不支持合成网格预览: <code>{type}</code></div>;
  }
  return (
    <div className="recipe-viewer">
      <div className="recipe-grid">
        {grid.map((row, ri) => (
          <div key={ri} className="recipe-row">
            {row.map((itemId, ci) => (
              <div key={ci} className={`recipe-slot ${itemId ? 'filled' : 'empty'}`} onClick={() => itemId && onSelect(itemId)} title={itemId?.startsWith('#') ? `${itemId}（标签；点击查看所有候选素材）` : itemId || ''}>
                {itemId && (
                  <>
                    {getItemIcon(itemId) && <img className="recipe-icon" src={getItemIcon(itemId)!} alt=""/>}
                    <span className="recipe-item-name">{translateKey(itemId)}</span>
                  </>
                )}
              </div>
            ))}
          </div>
        ))}
      </div>
      <div className="recipe-arrow">→</div>
      <div className="recipe-output">
        {output ? (
          <div className="recipe-slot filled output-slot" onClick={() => onSelect(output.id)} title={output.id}>
            {getItemIcon(output.id) && <img className="recipe-icon" src={getItemIcon(output.id)!} alt=""/>}
            <span className="recipe-item-name">{translateKey(output.id)}</span>
            {output.count > 1 && <span className="recipe-count">×{output.count}</span>}
          </div>
        ) : <div className="recipe-slot empty"><span className="recipe-item-name">无输出</span></div>}
      </div>
    </div>
  );
}
