import {useEffect, useState} from 'react';
import {App, Form, Input, Modal, Radio, Select, Tabs, Upload} from 'antd';
import {InboxOutlined} from '@ant-design/icons';
import {createPack, confirmImport, fetchMcVersions, inspectImport} from './api';
import type {ImportPreview, ImportSource} from './types';
import type {DashboardPack} from './types';

/* 新建 / 导入整合包对话框（看板页的直接子功能）。
   导入走后端真实的两阶段流程：先解析出预览，用户确认后才入队。 */

const loaders = [
  {value: 'forge', label: 'Forge', hint: '生态最老、模组最多'},
  {value: 'neoforge', label: 'NeoForge', hint: 'Forge 的现代分支，1.20.2+ 主流'},
  {value: 'fabric', label: 'Fabric', hint: '轻量、更新快'},
  {value: 'quilt', label: 'Quilt', hint: 'Fabric 的分支，社区驱动'},
];

export function CreatePackModal({open, existing, onClose, onCreated}: {
  open: boolean;
  existing: DashboardPack[];
  onClose: () => void;
  onCreated: (id: string) => void;
}) {
  const {message} = App.useApp();
  const [form] = Form.useForm();
  const [versions, setVersions] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) void fetchMcVersions().then(setVersions).catch(() => setVersions([]));
  }, [open]);

  const submit = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const created = await createPack(values);
      message.success(`已创建「${created.name}」`);
      form.resetFields();
      onCreated(created.id);
    } catch (err) {
      message.error(`创建失败：${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal title="新建整合包" open={open} onCancel={onClose} onOk={() => void submit()}
           confirmLoading={submitting} okText="创建" cancelText="取消" destroyOnHidden>
      <Form form={form} layout="vertical" preserve={false} initialValues={{loader: 'neoforge'}}>
        <Form.Item name="name" label="包名称" rules={[
          {required: true, message: '请输入包名称'},
          {max: 50, message: '不超过 50 字'},
          {validator: (_, v: string) => existing.some(p => p.name === v) ? Promise.reject(new Error('已存在同名整合包')) : Promise.resolve()},
        ]}>
          <Input placeholder="例如：我的冒险整合包" maxLength={50} showCount/>
        </Form.Item>
        <Form.Item name="mcVersion" label="MC 版本" rules={[{required: true, message: '请选择 MC 版本'}]}>
          <Select showSearch placeholder="选择或输入版本号" options={versions.map(v => ({value: v, label: v}))}/>
        </Form.Item>
        <Form.Item name="loader" label="加载器" rules={[{required: true}]}>
          <Radio.Group className="db-loader-option" style={{display: 'flex', flexDirection: 'column', gap: 8}}>
            {loaders.map(l => (
              <Radio.Button key={l.value} value={l.value} style={{width: '100%', textAlign: 'left'}}>
                <b>{l.label}</b> <span className="db-muted" style={{marginLeft: 8}}>{l.hint}</span>
              </Radio.Button>
            ))}
          </Radio.Group>
        </Form.Item>
        <Form.Item name="loaderVersion" label="加载器版本（选填）" rules={[{max: 50}]}>
          <Input placeholder="留空则由平台选择匹配的稳定版" maxLength={50}/>
        </Form.Item>
        <Form.Item name="description" label="包描述（选填）" rules={[{max: 200}]}>
          <Input.TextArea rows={3} maxLength={200} showCount placeholder="这个包的定位与玩法"/>
        </Form.Item>
        <div className="db-muted">版本与加载器创建后将成为整个包的作用域，修改将触发全包重解。</div>
      </Form>
    </Modal>
  );
}

/* 读取本地 zip 并转成 base64（去掉 data URL 前缀）。 */
function readAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result ?? '');
      resolve(result.slice(result.indexOf(',') + 1));
    };
    reader.onerror = () => reject(reader.error ?? new Error('读取文件失败'));
    reader.readAsDataURL(file);
  });
}

function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `web-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function ImportPackModal({open, onClose, onImported}: {
  open: boolean;
  onClose: () => void;
  onImported: (packId: string | null) => void;
}) {
  const {message} = App.useApp();
  const [url, setUrl] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [tab, setTab] = useState<ImportSource>('curseforge');
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [inspecting, setInspecting] = useState(false);
  const [confirming, setConfirming] = useState(false);

  const reset = () => {
    setUrl('');
    setFile(null);
    setPreview(null);
    setInspecting(false);
    setConfirming(false);
  };

  const close = () => {
    reset();
    onClose();
  };

  const inspect = async () => {
    if (tab !== 'local' && !url.trim()) {
      message.warning('请先粘贴链接');
      return;
    }
    if (tab === 'local' && !file) {
      message.warning('请先选择 zip 文件');
      return;
    }
    setInspecting(true);
    try {
      const contentBase64 = tab === 'local' && file ? await readAsBase64(file) : undefined;
      const result = await inspectImport({source: tab, url, contentBase64});
      setPreview(result);
    } catch (err) {
      message.error(`解析失败：${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setInspecting(false);
    }
  };

  const confirm = async () => {
    if (!preview) return;
    setConfirming(true);
    try {
      const result = await confirmImport(preview, newIdempotencyKey());
      message.success(`已开始导入「${preview.packName || '整合包'}」，进度见后台任务`);
      reset();
      onImported(result.packId);
    } catch (err) {
      message.error(`导入失败：${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setConfirming(false);
    }
  };

  const linkForm = (platform: string) => (
    <>
      <Input placeholder={`粘贴 ${platform} 整合包链接`} value={url} onChange={e => setUrl(e.target.value)}/>
      <div className="db-muted" style={{marginTop: 8}}>解析出包名和条目数后会先给你预览，确认后再导入。</div>
    </>
  );

  const previewView = preview && (
    <>
      <div className="db-card" style={{padding: 16}}>
        <div className="db-h2">{preview.packName || '未命名整合包'}</div>
        <div className="db-muted" style={{marginTop: 8}}>
          来源：{preview.source} · 解析出 {preview.entryCount} 个条目
        </div>
        {preview.expiresAt && (
          <div className="db-muted" style={{marginTop: 4}}>预览有效期至 {new Date(preview.expiresAt).toLocaleString()}</div>
        )}
      </div>
      <div className="db-muted" style={{marginTop: 12}}>确认后会在后台任务中执行导入，可以在右侧任务面板查看进度。</div>
    </>
  );

  return (
    <Modal
      title="导入整合包"
      open={open}
      onCancel={close}
      destroyOnHidden
      footer={preview ? [
        <button key="back" className="db-link" style={{marginRight: 12}} onClick={() => setPreview(null)}>返回修改</button>,
        <button key="cancel" className="db-link" style={{marginRight: 12}} onClick={close}>取消</button>,
        <button key="ok" className="mc-btn-cta" disabled={confirming} onClick={() => void confirm()}>
          {confirming ? '提交中…' : '确认导入'}
        </button>,
      ] : [
        <button key="cancel" className="db-link" style={{marginRight: 12}} onClick={close}>取消</button>,
        <button key="ok" className="mc-btn-cta" disabled={inspecting} onClick={() => void inspect()}>
          {inspecting ? '解析中…' : '解析'}
        </button>,
      ]}
    >
      {preview ? previewView : (
        <Tabs activeKey={tab} onChange={k => setTab(k as ImportSource)} items={[
          {key: 'curseforge', label: 'CurseForge 链接', children: linkForm('CurseForge')},
          {key: 'modrinth', label: 'Modrinth 链接', children: linkForm('Modrinth')},
          {key: 'local', label: '本地 zip 文件', children: (
            <Upload.Dragger accept=".zip" maxCount={1} beforeUpload={f => {setFile(f); return false;}}
                            onRemove={() => setFile(null)}>
              <p><InboxOutlined style={{fontSize: 32, color: 'var(--mc-blue)'}}/></p>
              <p>拖拽 zip 到这里，或点击选择文件</p>
            </Upload.Dragger>
          )},
        ]}/>
      )}
    </Modal>
  );
}
