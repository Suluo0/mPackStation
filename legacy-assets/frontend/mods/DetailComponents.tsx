import type {ReactNode} from 'react';
import {Descriptions, Divider, List, Skeleton, Space, Tag, Typography} from 'antd';
import {DownloadOutlined, LinkOutlined} from '@ant-design/icons';
import type {ModDetail} from '../../types/api';

export type DetailField = {label: string; value: ReactNode};
export type PlatformState = 'success' | 'failed' | 'not_attempted' | 'processing';

export function PlatformStatusTag({label, value}: {label: string; value: PlatformState}) {
  return <Tag color={value === 'success' ? 'green' : value === 'failed' ? 'red' : value === 'not_attempted' ? 'default' : 'orange'}>{label} · {value === 'success' ? '已获取' : value === 'failed' ? '失败' : value === 'not_attempted' ? '未尝试' : '处理中'}</Tag>;
}

export function DetailFieldGrid({fields, columns = 3}: {fields: DetailField[]; columns?: 1 | 2 | 3}) {
  return <Descriptions className="mod-detail-descriptions" size="small" column={columns}>{fields.map(field => <Descriptions.Item key={field.label} label={field.label}>{field.value ?? '—'}</Descriptions.Item>)}</Descriptions>;
}

export function DetailSection({title, children}: {title: string; children: ReactNode}) {
  return <section className="detail-section"><Divider>{title}</Divider>{children}</section>;
}

export function DetailSyncSkeleton() {
  return <div className="detail-sync-box"><Skeleton active title paragraph={{rows: 2}}/></div>;
}

export function ModDetailHeader({name, nameEn, modId, status, relationLabel, sourceLabel, actions}: {name: string; nameEn?: string | null; modId: string; status?: string | null; relationLabel: string; sourceLabel?: string; actions?: ReactNode}) {
  return <div className="detail-title">
    <div><Typography.Title level={3}>{name || nameEn || modId}</Typography.Title><Typography.Text type="secondary">{nameEn || '—'} · {modId}</Typography.Text></div>
    <Space direction="vertical" align="end">
      <Tag color={status === 'installed' ? 'green' : 'blue'}>{status === 'installed' ? '全局状态：installed' : `全局状态：${status || '未记录'}`}</Tag>
      <Tag color={relationLabel.includes('未关联') ? 'default' : 'blue'}>{relationLabel}</Tag>
      {sourceLabel && <Tag color={sourceLabel.includes('服务器') ? 'blue' : 'gold'}>{sourceLabel}</Tag>}
      {actions}
    </Space>
  </div>;
}

const formatBytes = (size?: number | null) => size == null ? '—' : `${(size / 1024 / 1024).toFixed(2)} MB`;

export function ModDetailBody({detail, platformContent, extraTop}: {detail: ModDetail; platformContent?: ReactNode; extraTop?: ReactNode}) {
  const hasPlatformLink = (platform: string) => detail.links.some(link => link.platform.toLowerCase() === platform);
  const platform = platformContent ?? <Space wrap>
    <Tag color={hasPlatformLink('modrinth') ? 'green' : 'default'}>MR · {hasPlatformLink('modrinth') ? '已获取' : '未尝试'}</Tag>
    <Tag color={hasPlatformLink('curseforge') ? 'green' : 'default'}>CF · {hasPlatformLink('curseforge') ? '已获取' : '未尝试'}</Tag>
  </Space>;
  const sections = [['summary', '简介'], ['content', '主要内容'], ['compatibility', '兼容性'], ['dependencies', '依赖'], ['evaluation', '评估'], ['sources', '来源（旧契约）']] as const;
  return <>
    {extraTop}
    <DetailSection title="基础信息"><DetailFieldGrid fields={[{label: '模组 ID', value: detail.mod.modId}, {label: '英文名称', value: detail.mod.nameEn}, {label: '中文名称', value: detail.mod.nameZh}, {label: '状态', value: detail.mod.status}, {label: '数据修订', value: `revision ${detail.mod.revision ?? '—'}`}, {label: '平台入口数', value: detail.links.length || '—'}]} /></DetailSection>
    <DetailSection title="平台地址"><Space wrap>{platform}{detail.links.length ? detail.links.map(link => link.url && <Typography.Link key={`${link.platform}-${link.url}`} href={link.url} target="_blank" rel="noreferrer"><LinkOutlined/> {link.platform}</Typography.Link>) : <Typography.Text type="secondary">暂无平台链接</Typography.Text>}</Space></DetailSection>
    {sections.map(([key, title]) => <DetailSection title={title} key={key}><Typography.Paragraph className="detail-prose">{detail.details?.[key] || (key === 'sources' ? '未记录；待迁移后的来源审计字段' : '—')}</Typography.Paragraph></DetailSection>)}
    <DetailSection title="可下载文件（mod_files）"><List size="small" dataSource={detail.files} locale={{emptyText: '暂无精确文件记录'}} renderItem={file => <List.Item actions={file.downloads.map(url => <Typography.Link key={url} href={url} target="_blank" rel="noreferrer"><DownloadOutlined/> 下载</Typography.Link>)}><List.Item.Meta title={<Space><Typography.Text strong>{file.filename || '—'}</Typography.Text><Tag>{file.platform || '—'}</Tag></Space>} description={<span>{file.fileKey || '—'} · {formatBytes(file.fileSize)} · SHA-1 {file.sha1 || '—'}</span>}/></List.Item>}/></DetailSection>
  </>;
}
