import {useMemo, useState} from 'react';
import {Button, Input, Select, Space, Tag, Typography} from 'antd';

type Controller = {x?: number; y?: number; z?: number};
type Props = {sizeX: number; sizeY: number; sizeZ: number; structure: Record<string, unknown>; controller?: Controller; onChangeStructure?: (structure: Record<string, unknown>) => void; onChangeController?: (controller: Controller) => void};
type Block = {x: number; y: number; z: number; value: string; active: boolean};

function blockName(value: unknown) {
  const text = String(value || 'minecraft:air');
  return text === 'air' || text.endsWith(':air') ? '' : (text.split(':').pop() || text).replaceAll('_', ' ');
}

/** A compact isometric 3D explorer. It renders all occupied layers on demand and keeps the editor's JSON as the source of truth. */
export function IsoPreview({sizeX, sizeY, sizeZ, structure, controller, onChangeStructure, onChangeController}: Props) {
  const [layerIndex, setLayerIndex] = useState(0);
  const [showAllLayers, setShowAllLayers] = useState(false);
  const [rotation, setRotation] = useState(0);
  const [selected, setSelected] = useState<Block | null>(null);
  const [editValue, setEditValue] = useState('');
  const [dragSource, setDragSource] = useState<Block | null>(null);
  const layers = useMemo(() => Array.isArray(structure.layers) ? structure.layers as unknown[][][] : [], [structure]);
  const safeLayer = Math.min(Math.max(layerIndex, 0), Math.max(0, sizeY - 1));
  const blocks = useMemo(() => {
    const result: Block[] = [];
    const yValues = showAllLayers ? Array.from({length: Math.max(1, sizeY)}, (_, y) => y) : [safeLayer];
    for (const y of yValues) {
      const layer = Array.isArray(layers[y]) ? layers[y] : [];
      for (let z = 0; z < Math.max(1, sizeZ); z += 1) {
        const row = Array.isArray(layer[z]) ? layer[z] : [];
        for (let x = 0; x < Math.max(1, sizeX); x += 1) {
          const value = blockName(row[x]);
          const active = x === controller?.x && y === (controller?.y ?? 0) && z === controller?.z;
          if (value || active || onChangeStructure) result.push({x, y, z, value, active});
        }
      }
    }
    return result;
  }, [controller?.x, controller?.y, controller?.z, layers, onChangeStructure, safeLayer, showAllLayers, sizeX, sizeY, sizeZ]);
  const occupiedCount = blocks.filter(block => block.value || block.active).length;
  const renderedBlocks = blocks.slice(0, 1200);
  const planeWidth = Math.max(150, sizeX * 34 + sizeY * 16);
  const planeHeight = Math.max(150, sizeZ * 34 + sizeY * 16);
  const rotate = (delta: number) => setRotation(value => (value + delta + 360) % 360);
  const selectBlock = (block: Block) => {setSelected(block); setEditValue(block.value || 'air');};
  const writeSelected = () => {
    if (!selected || !onChangeStructure) return;
    const next = JSON.parse(JSON.stringify(structure)) as {layers?: unknown[][][]};
    const nextLayers = Array.isArray(next.layers) ? next.layers : [];
    const nextLayer = Array.isArray(nextLayers[selected.y]) ? nextLayers[selected.y] : [];
    const nextRow = Array.isArray(nextLayer[selected.z]) ? nextLayer[selected.z] : [];
    while (nextRow.length <= selected.x) nextRow.push('air');
    nextRow[selected.x] = editValue.trim() || 'air';
    nextLayer[selected.z] = nextRow; nextLayers[selected.y] = nextLayer; next.layers = nextLayers;
    onChangeStructure(next); setSelected({...selected, value: blockName(editValue)});
  };
  const moveBlock = (target: Block) => {
    if (!dragSource || target.active || (dragSource.x === target.x && dragSource.y === target.y && dragSource.z === target.z)) return;
    if (dragSource.active && onChangeController) { onChangeController({...controller, x: target.x, y: target.y, z: target.z}); setSelected({...target, active: true, value: 'controller'}); setDragSource(null); return; }
    if (!onChangeStructure || dragSource.active) return;
    const next = JSON.parse(JSON.stringify(structure)) as {layers?: unknown[][][]};
    const nextLayers = Array.isArray(next.layers) ? next.layers : [];
    const sourceLayer = Array.isArray(nextLayers[dragSource.y]) ? nextLayers[dragSource.y] : [];
    const targetLayer = Array.isArray(nextLayers[target.y]) ? nextLayers[target.y] : [];
    const sourceRow = Array.isArray(sourceLayer[dragSource.z]) ? sourceLayer[dragSource.z] : [];
    const targetRow = Array.isArray(targetLayer[target.z]) ? targetLayer[target.z] : [];
    while (sourceRow.length <= dragSource.x) sourceRow.push('air'); while (targetRow.length <= target.x) targetRow.push('air');
    const sourceValue = sourceRow[dragSource.x] ?? 'air'; const targetValue = targetRow[target.x] ?? 'air';
    sourceRow[dragSource.x] = targetValue; targetRow[target.x] = sourceValue;
    sourceLayer[dragSource.z] = sourceRow; targetLayer[target.z] = targetRow; nextLayers[dragSource.y] = sourceLayer; nextLayers[target.y] = targetLayer; next.layers = nextLayers;
    onChangeStructure(next); setSelected({...target, value: blockName(sourceValue)}); setDragSource(null);
  };
  return <div className="iso-preview-wrap">
    <Space size="small" wrap>
      <Typography.Text strong>立体预览</Typography.Text>
      <Button size="small" onClick={() => setShowAllLayers(value => !value)}>{showAllLayers ? '仅看当前层' : '显示全部层'}</Button>
      {!showAllLayers && <Select size="small" value={safeLayer} onChange={value => {setLayerIndex(value); setSelected(null);}} options={Array.from({length: Math.max(1, sizeY)}, (_, y) => ({value: y, label: `Y=${y}`}))}/>} 
      <Button size="small" onClick={() => rotate(-90)} aria-label="逆时针旋转">↶</Button>
      <Button size="small" onClick={() => rotate(90)} aria-label="顺时针旋转">↷</Button>
      <Tag color="blue">{occupiedCount} 个非空气方块{blocks.length > renderedBlocks.length ? '（编辑网格已限制显示）' : ''}</Tag>
    </Space>
    <div className="iso-preview" aria-label={showAllLayers ? '多方块全部层立体预览' : `多方块第 ${safeLayer} 层立体预览`}>
      <div className="iso-plane" style={{width: planeWidth, height: planeHeight, transform: `rotateX(57deg) rotateZ(${rotation - 45}deg)`}}>
        {renderedBlocks.map(block => <button type="button" draggable={Boolean((onChangeStructure || onChangeController) && (block.active || block.value))} key={`${block.x}-${block.y}-${block.z}`} className={`iso-block ${!block.value && !block.active ? 'iso-block-air' : ''} ${block.active ? 'iso-block-controller' : ''} ${selected?.x === block.x && selected.y === block.y && selected.z === block.z ? 'iso-block-selected' : ''}`} style={{left: `${block.x * 34 + block.y * 16 + 8}px`, top: `${block.z * 34 + (sizeY - block.y - 1) * 16 + 8}px`, transform: `translateZ(${block.y * 18 + 12}px)`}} onClick={() => selectBlock(block)} onDragStart={() => setDragSource(block)} onDragOver={event => event.preventDefault()} onDrop={() => moveBlock(block)} title={`${block.x},${block.y},${block.z}: ${block.value || 'air'}`}>{block.active ? 'C' : block.value.slice(0, 4)}</button>)}
      </div>
    </div>
    {selected && <Space className="iso-selection" size="small" wrap><Typography.Text type="secondary">已选方块：({selected.x}, {selected.y}, {selected.z}) · {selected.value || 'controller'}</Typography.Text>{onChangeStructure && <><Input size="small" value={editValue} onChange={event => setEditValue(event.target.value)} style={{width: 190}} aria-label="3D 方块 ID"/><Button size="small" type="primary" onClick={writeSelected}>写入当前草稿</Button></>}</Space>}
    <Typography.Text type="secondary" className="iso-preview-hint">可旋转视角、切换单层/全部层并点击方块查看坐标；编辑模式下可拖动方块或控制器定位，保存后生效。</Typography.Text>
  </div>;
}
