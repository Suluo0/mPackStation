import {useEffect, useState} from 'react';
import {App, Button, Input, Modal, Spin} from 'antd';
import {FolderOpenOutlined, HomeOutlined, UpOutlined} from '@ant-design/icons';
import {browseDirectories, type FsBrowse} from '../../api/fs';

/* 本机目录选择器：通过后端 /api/fs/browse 枚举绝对路径。
   纯文本输入只能填路径；这里提供真实可点的目录选择。 */
export function DirectoryPicker({
  open,
  title = '选择目录',
  value,
  onClose,
  onSelect,
}: {
  open: boolean;
  title?: string;
  value?: string;
  onClose: () => void;
  onSelect: (path: string) => void;
}) {
  const {message} = App.useApp();
  const [path, setPath] = useState('');
  const [manual, setManual] = useState('');
  const [data, setData] = useState<FsBrowse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setManual(value || '');
    void load(value || '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const load = async (target: string) => {
    setLoading(true);
    setError('');
    try {
      const res = await browseDirectories(target || undefined);
      setData(res);
      setPath(res.path);
      if (!manual) setManual(res.path);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setData(null);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal
      title={title}
      open={open}
      onCancel={onClose}
      width={640}
      footer={
        <div style={{display: 'flex', justifyContent: 'space-between', gap: 8}}>
          <Button icon={<HomeOutlined/>} onClick={() => void load('')}>主目录</Button>
          <div style={{display: 'flex', gap: 8}}>
            <Button onClick={onClose}>取消</Button>
            <Button
              type="primary"
              icon={<FolderOpenOutlined/>}
              onClick={() => {
                const pick = (manual || path || '').trim();
                if (!pick) {
                  message.error('请选择或填写目录路径');
                  return;
                }
                onSelect(pick);
                onClose();
              }}
           >
              使用此目录
            </Button>
          </div>
        </div>
      }
    >
      <div className="dir-picker">
        <div className="dir-picker-path">
          <Button
            size="small"
            icon={<UpOutlined/>}
            disabled={!data?.parent}
            onClick={() => data?.parent && void load(data.parent)}
          >
            上级
          </Button>
          <div className="dir-picker-current">
            <span className="dir-picker-current-label">当前路径</span>
            <code>{path || '—'}</code>
          </div>
        </div>
        <div className="dir-picker-manual">
          <Input
            placeholder="也可手动填写绝对路径"
            value={manual}
            onChange={e => setManual(e.target.value)}
          />
        </div>
        {error && <div className="empty-inline">加载失败：{error}</div>}
        {loading && <div className="dir-picker-loading"><Spin/></div>}
        {!loading && data && (
          <>
            <div className="dir-picker-label">推荐位置</div>
            <div className="dir-picker-list">
              {data.suggested.map(s => (
                <button key={s.path} type="button" className="dir-picker-item" onClick={() => { setManual(s.path); void load(s.path); }}>
                  <FolderOpenOutlined/>
                  <span>{s.name}</span>
                  <small>{s.path}</small>
                </button>
              ))}
            </div>
            <div className="dir-picker-label">子目录（{data.directories.length}）</div>
            <div className="dir-picker-list dir-picker-scroll">
              {data.directories.length === 0 && <div className="empty-inline">该目录下没有可浏览的子目录，可手动填写路径。</div>}
              {data.directories.map(d => (
                <button key={d.path} type="button" className="dir-picker-item" onClick={() => { setManual(d.path); void load(d.path); }}>
                  <FolderOpenOutlined/>
                  <span>{d.name}</span>
                  <small>{d.path}</small>
                </button>
              ))}
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}
