import {useState} from 'react';
import {Button} from 'antd';
import {CloseOutlined, WarningFilled} from '@ant-design/icons';
import {useNavigate} from 'react-router-dom';
import type {SystemHealth} from './types';
import {formatBytes} from './signals';

/* 环境自检条：仅在发现问题时渲染，可关闭（本次会话不再出现）。 */

const FIVE_GB = 5 * 1024 ** 3;

type Issue = {key: string; text: string; action: {label: string; to: string}};

function detectIssues(health: SystemHealth): Issue[] {
  const issues: Issue[] = [];
  if (!health.curseforgeKeyConfigured) {
    issues.push({key: 'cf-key', text: 'CurseForge API Key 未配置，将无法检索 CurseForge 模组。', action: {label: '去设置', to: '/settings'}});
  }
  if (health.modrinthStatus === 'unavailable') {
    issues.push({key: 'mr-down', text: 'Modrinth 当前无法连接，搜索将只返回 CurseForge 结果。', action: {label: '重试', to: ''}});
  }
  if (health.curseforgeStatus === 'unavailable') {
    issues.push({key: 'cf-down', text: 'CurseForge 当前无法连接，搜索将只返回 Modrinth 结果。', action: {label: '重试', to: ''}});
  }
  if (!health.storageWritable) {
    issues.push({key: 'storage-ro', text: '存储目录不可写，无法下载模组。', action: {label: '查看存储设置', to: '/settings'}});
  } else if (health.storageFreeBytes < FIVE_GB) {
    issues.push({key: 'storage-low', text: `存储空间不足（剩余 ${formatBytes(health.storageFreeBytes)}），可能无法下载模组。`, action: {label: '查看存储设置', to: '/settings'}});
  }
  return issues;
}

export function EnvHealthBanner({health, onRetry, suppressKeys = []}: {health: SystemHealth; onRetry: () => void; suppressKeys?: string[]}) {
  const navigate = useNavigate();
  const [dismissed, setDismissed] = useState<string[]>([]);
  const issues = detectIssues(health).filter(i => !dismissed.includes(i.key) && !suppressKeys.includes(i.key));
  if (issues.length === 0) return null;
  return (
    <>
      {issues.map(issue => (
        <div key={issue.key} className="db-banner" role="alert">
          <WarningFilled/>
          <span className="db-banner-text">{issue.text}</span>
          <button className="db-link" onClick={() => (issue.action.to ? navigate(issue.action.to) : onRetry())}>
            {issue.action.label}
          </button>
          <Button type="text" size="small" className="db-banner-close" icon={<CloseOutlined/>}
                  onClick={() => setDismissed(prev => [...prev, issue.key])}/>
        </div>
      ))}
    </>
  );
}
