import type {MouseEventHandler, ReactNode} from 'react';
import {Button, type ButtonProps} from 'antd';

type CardProps = {
  children: ReactNode;
  className?: string;
  as?: 'div' | 'section';
  onClick?: MouseEventHandler<HTMLElement>;
};

export function WorkbenchCard({children, className = '', as = 'div', onClick}: CardProps) {
  const Tag = as;
  return <Tag className={`wb-card ${className}`.trim()} onClick={onClick}>{children}</Tag>;
}

export function WorkbenchSectionHeader({title, action}: {title: ReactNode; action?: ReactNode}) {
  return <div className="wb-section-head"><h2 className="wb-title">{title}</h2>{action}</div>;
}

type WorkbenchButtonProps = ButtonProps & {tone?: 'primary' | 'secondary' | 'quiet'};

export function WorkbenchButton({tone = 'secondary', className = '', type, ...props}: WorkbenchButtonProps) {
  const toneClass = tone === 'primary' ? 'wb-btn-primary mc-btn-cta' : tone === 'quiet' ? 'wb-btn-quiet' : 'wb-btn-secondary';
  return <Button {...props} type={type ?? (tone === 'primary' ? 'primary' : 'default')} className={`${toneClass} ${className}`.trim()} />;
}
