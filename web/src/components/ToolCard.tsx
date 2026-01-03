import { useCallback, useMemo, useState, cloneElement } from 'react';
import type { CSSProperties, ReactNode, ReactElement } from 'react';
import { Card, Space, Tag, Button, Tooltip, Collapse, Typography, Descriptions, Progress, Spin, Dropdown } from 'antd';
import type { MenuProps, ProgressProps } from 'antd';
import type { MessageInstance } from 'antd/es/message/interface';
import { DownloadOutlined, PauseCircleOutlined, PlayCircleOutlined, RedoOutlined, DeleteOutlined, ReloadOutlined, CopyOutlined, InfoCircleOutlined, FolderOpenOutlined } from '@ant-design/icons';
import type { ToolInfo, ToolRuntimeStatus, ToolFoldersResponse } from '@/utils/api';
import { fetchToolMetadata, fetchToolInfoText, fetchToolFolders } from '@/utils/api';
import { formatBytes, formatSpeed } from '@/utils/format';
import type { ProgressMessage } from '@/utils/api';
import { useLocale } from '@/hooks/useLocale';
import { JsonPreview } from '@/components/JsonPreview';
import type { ResolvedProgress, ToolStatusKey } from '@/utils/toolStatus';
import { resolveProgress, deriveToolStatus, hasActiveDownload, isResumableStatus, isInstallableStatus, isInstalledStatus, formatProgressSummary } from '@/utils/toolStatus';

const { Panel } = Collapse;

interface ToolCardProps {
  tool: ToolInfo;
  runtime?: ToolRuntimeStatus;
  progress?: ProgressMessage;
  resolved?: ResolvedProgress;
  status?: ToolStatusKey;
  onInstall: (toolName: string, version: string) => Promise<void>;
  onPause: (toolName: string, version: string) => Promise<void>;
  onResume: (toolName: string, version: string) => Promise<void>;
  onUninstall: (toolName: string, version: string) => Promise<void>;
  messageApi: MessageInstance;
}

type TabKey = 'metadata' | 'info' | 'folders';

function statusTagColor(status: ToolStatusKey): string {
  switch (status) {
    case 'downloading':
    case 'trying':
      return 'blue';
    case 'extracting':
      return 'gold';
    case 'completed':
      return 'green';
    case 'paused':
      return 'purple';
    case 'failed':
      return 'red';
    default:
      return 'default';
  }
}

interface FloatingActionIconProps {
  tooltip: string;
  icon: ReactNode;
  onClick: () => void;
  style?: CSSProperties;
  disabled?: boolean;
  loading?: boolean;
}

function FloatingActionIcon({ tooltip, icon, onClick, style, disabled, loading }: FloatingActionIconProps) {
  return (
    <Tooltip title={tooltip}>
      <Button
        type="text"
        shape="circle"
        size="small"
        icon={icon}
        onClick={onClick}
        disabled={disabled}
        loading={loading}
        className="panel-floating-btn"
        style={style}
      />
    </Tooltip>
  );
}

function stripEnabledField(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(stripEnabledField);
  }
  if (value && typeof value === 'object') {
    const result: Record<string, unknown> = {};
    for (const [key, val] of Object.entries(value as Record<string, unknown>)) {
      if (key === 'enabled') {
        continue;
      }
      result[key] = stripEnabledField(val);
    }
    return result;
  }
  return value;
}

function sanitizeMetadata(raw: string): { text: string; valid: boolean } {
  if (!raw) {
    return { text: '', valid: true };
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    const cleaned = stripEnabledField(parsed);
    return { text: JSON.stringify(cleaned, null, 2), valid: true };
  } catch (error) {
    console.error('sanitize metadata failed', error);
    return { text: raw, valid: false };
  }
}

export function ToolCard({
  tool,
  runtime,
  progress,
  resolved,
  status: statusProp,
  onInstall,
  onPause,
  onResume,
  onUninstall,
  messageApi
}: ToolCardProps) {
  const { t } = useLocale();

  const initialSanitized = useMemo(() => sanitizeMetadata(tool.metadataJson ?? ''), [tool.metadataJson]);
  const [metadata, setMetadata] = useState(initialSanitized.text);
  const [metadataValid, setMetadataValid] = useState(initialSanitized.valid);
  const [metadataLoading, setMetadataLoading] = useState(false);
  const [info, setInfo] = useState('');
  const [infoLoading, setInfoLoading] = useState(false);
  const [folders, setFolders] = useState<ToolFoldersResponse | null>(null);
  const [foldersLoading, setFoldersLoading] = useState(false);
  const [activeTab, setActiveTab] = useState<TabKey | null>(null);

  const resolvedProgress = useMemo(
    () => resolved ?? resolveProgress(tool, runtime, progress),
    [resolved, tool, runtime, progress]
  );
  const currentStatus = useMemo(
    () => statusProp ?? deriveToolStatus(tool, runtime, progress),
    [statusProp, tool, runtime, progress]
  );

  const handleInstall = useCallback(() => {
    void onInstall(tool.name, tool.version);
  }, [onInstall, tool.name, tool.version]);

  const handlePause = useCallback(() => {
    void onPause(tool.name, tool.version);
  }, [onPause, tool.name, tool.version]);

  const handleResume = useCallback(() => {
    void onResume(tool.name, tool.version);
  }, [onResume, tool.name, tool.version]);

  const handleUninstall = useCallback(() => {
    void onUninstall(tool.name, tool.version);
  }, [onUninstall, tool.name, tool.version]);

  const handleLoadMetadata = useCallback(async () => {
    setMetadataLoading(true);
    try {
      const latest = await fetchToolMetadata(tool.name, tool.version);
      const sanitized = sanitizeMetadata(latest);
      setMetadata(sanitized.text);
      setMetadataValid(sanitized.valid);
    } catch (error) {
      console.error('load metadata failed', error);
      messageApi.error(t('metadataFailed'));
    } finally {
      setMetadataLoading(false);
    }
  }, [messageApi, t, tool.name, tool.version]);

  const handleCopyMetadata = useCallback(async () => {
    if (!metadata) {
      return;
    }
    try {
      await navigator.clipboard.writeText(metadata);
      messageApi.success(t('copySuccess'));
    } catch (error) {
      console.error('copy metadata failed', error);
      messageApi.error(t('copyFailed'));
    }
  }, [metadata, messageApi, t]);

  const handleLoadInfo = useCallback(async () => {
    setInfoLoading(true);
    try {
      const text = await fetchToolInfoText(tool.name, tool.version);
      setInfo(text);
    } catch (error) {
      console.error('load info failed', error);
      messageApi.error(t('infoFailed'));
    } finally {
      setInfoLoading(false);
    }
  }, [messageApi, t, tool.name, tool.version]);

  const handleLoadFolders = useCallback(async () => {
    setFoldersLoading(true);
    try {
      const data = await fetchToolFolders(tool.name, tool.version);
      setFolders(data);
    } catch (error) {
      console.error('load folders failed', error);
      messageApi.error(t('folderFailed'));
    } finally {
      setFoldersLoading(false);
    }
  }, [messageApi, t, tool.name, tool.version]);

  const handleTabChange = useCallback(
    (key: string | string[]) => {
      if (!key || (Array.isArray(key) && key.length === 0)) {
        setActiveTab(null);
        return;
      }
      const nextKey = Array.isArray(key) ? (key[0] as TabKey) : (key as TabKey);
      setActiveTab(nextKey);
      if (nextKey === 'metadata' && !metadata) {
        void handleLoadMetadata();
      }
      if (nextKey === 'info' && !info) {
        void handleLoadInfo();
      }
      if (nextKey === 'folders' && !folders) {
        void handleLoadFolders();
      }
    },
    [metadata, info, folders, handleLoadMetadata, handleLoadInfo, handleLoadFolders]
  );

  const uninstallMenu = useMemo<MenuProps['items']>(() => {
    if (tool.preinstalled) {
      return [] as MenuProps['items'];
    }
    return [
      {
        key: 'uninstall',
        label: <span style={{ color: '#ff4d4f' }}>{t('uninstall')}</span>,
        icon: <DeleteOutlined style={{ color: '#ff4d4f' }} />,
      }
    ] as MenuProps['items'];
  }, [tool.preinstalled, t]);

  const handleMenuClick = useCallback<NonNullable<MenuProps['onClick']>>(
    ({ key }) => {
      if (key === 'uninstall') {
        handleUninstall();
      }
    },
    [handleUninstall]
  );

  // 判断是否显示独立的卸载按钮（在暂停、失败、下载中等非完成状态下也允许卸载以便用户恢复）
  const showStandaloneUninstall = useMemo(() => {
    if (tool.preinstalled) {
      return false;
    }
    // 已安装状态由 Dropdown.Button 处理卸载，不需要独立按钮
    if (isInstalledStatus(currentStatus)) {
      return false;
    }
    // 其他状态（暂停、失败、下载中、解压中等）显示独立卸载按钮以便用户重置
    return true;
  }, [tool.preinstalled, currentStatus]);

  const primaryAction = useMemo<ReactNode>(() => {
    if (hasActiveDownload(currentStatus)) {
      return (
        <Button type="default" icon={<PauseCircleOutlined />} onClick={handlePause} disabled={tool.preinstalled}>
          {t('pause')}
        </Button>
      );
    }
    if (isResumableStatus(currentStatus)) {
      return (
        <Button type="primary" icon={<PlayCircleOutlined />} onClick={handleResume} disabled={tool.preinstalled}>
          {t('resume')}
        </Button>
      );
    }
    if (isInstalledStatus(currentStatus)) {
      if (tool.preinstalled || !(uninstallMenu && uninstallMenu.length)) {
        return null;
      }
      return (
        <Dropdown.Button
          type="primary"
          menu={{ items: uninstallMenu, onClick: handleMenuClick }}
          onClick={handleInstall}
          buttonsRender={([left, right]) => [
            cloneElement(left as ReactElement, { icon: <RedoOutlined /> }),
            right,
          ]}
        >
          {t('reinstall')}
        </Dropdown.Button>
      );
    }
    if (isInstallableStatus(currentStatus)) {
      return (
        <Button type="primary" icon={<DownloadOutlined />} onClick={handleInstall} disabled={tool.preinstalled}>
          {tool.installed ? t('reinstall') : t('install')}
        </Button>
      );
    }
    return (
      <Button type="primary" icon={<DownloadOutlined />} onClick={handleInstall} disabled={tool.preinstalled}>
        {t('install')}
      </Button>
    );
  }, [currentStatus, handleInstall, handleMenuClick, handlePause, handleResume, t, tool.installed, tool.preinstalled, uninstallMenu]);

  // 只在活跃下载或暂停中才显示进度信息
  const showProgress = hasActiveDownload(currentStatus) || isResumableStatus(currentStatus);
  const etaText = resolvedProgress.eta ?? (hasActiveDownload(currentStatus) ? t('etaCalculating') : undefined);
  const progressStatus: ProgressProps['status'] =
    currentStatus === 'failed' ? 'exception' : hasActiveDownload(currentStatus) ? 'active' : 'normal';

  return (
    <Card
      title={
        <Space size={8} wrap>
          <Typography.Text strong>{tool.name}</Typography.Text>
          <Tag color="geekblue">{tool.version}</Tag>
          {tool.execFromTemp && <Tag color="purple">{t('execFromTemp')}</Tag>}
          {!tool.isExecutable && <Tag color="volcano">{t('nonExecutable')}</Tag>}
        </Space>
      }
      extra={
        <Space size={8} wrap>
          {tool.preinstalled && <Tag color="gold">{t('preinstalled')}</Tag>}
          <Tag color={statusTagColor(currentStatus)}>{t(currentStatus)}</Tag>
        </Space>
      }
      bordered
      style={{ minHeight: 320 }}
    >
      <Space direction="vertical" size="large" style={{ width: '100%' }}>
        {showProgress && (
          <>
            <Space direction="vertical" size={4} style={{ width: '100%' }}>
              <Progress percent={Math.round(resolvedProgress.percent)} status={progressStatus} />
              <Typography.Text type="secondary">{formatProgressSummary(resolvedProgress)}</Typography.Text>
            </Space>

            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label={t('downloadBytes')}>
                {formatBytes(resolvedProgress.downloadedBytes)} / {formatBytes(resolvedProgress.totalBytes)}
              </Descriptions.Item>
              <Descriptions.Item label={t('speed')}>{formatSpeed(resolvedProgress.speed)}</Descriptions.Item>
              {etaText && <Descriptions.Item label={t('eta')}>{etaText}</Descriptions.Item>}
              {resolvedProgress.currentUrl && (
                <Descriptions.Item label="URL">
                  <Typography.Link href={resolvedProgress.currentUrl} target="_blank" rel="noopener noreferrer">
                    {resolvedProgress.currentUrl}
                  </Typography.Link>
                </Descriptions.Item>
              )}
              {!!resolvedProgress.failedUrls?.length && (
                <Descriptions.Item label={t('mirrorStatusFailed')}>
                  <Space direction="vertical" size={4}>
                    {resolvedProgress.failedUrls.map((url) => (
                      <Typography.Link key={url} href={url} target="_blank" rel="noopener noreferrer">
                        {url}
                      </Typography.Link>
                    ))}
                  </Space>
                </Descriptions.Item>
              )}
            </Descriptions>
          </>
        )}

        {primaryAction && (
          <Space>
            {primaryAction}
            {showStandaloneUninstall && (
              <Button
                danger
                icon={<DeleteOutlined />}
                onClick={handleUninstall}
              >
                {t('uninstall')}
              </Button>
            )}
          </Space>
        )}
        {!primaryAction && showStandaloneUninstall && (
          <Space>
            <Button
              danger
              icon={<DeleteOutlined />}
              onClick={handleUninstall}
            >
              {t('uninstall')}
            </Button>
          </Space>
        )}

        <Collapse accordion activeKey={activeTab ? [activeTab] : []} onChange={handleTabChange}>
          <Panel header={t('metadata')} key="metadata">
            <div style={{ position: 'relative' }}>
              <FloatingActionIcon
                tooltip={t('refreshMetadata')}
                icon={<ReloadOutlined />}
                onClick={handleLoadMetadata}
                loading={metadataLoading}
              />
              <FloatingActionIcon
                tooltip={t('copyMetadata')}
                icon={<CopyOutlined />}
                onClick={handleCopyMetadata}
                disabled={!metadata}
                style={{ right: 44 }}
              />
              <Spin spinning={metadataLoading}>
                {metadata ? (
                  <>
                    {!metadataValid && (
                      <Typography.Paragraph>
                        <Typography.Text type="warning">{t('metadataParseFailed')}</Typography.Text>
                      </Typography.Paragraph>
                    )}
                    <JsonPreview json={metadata} />
                  </>
                ) : (
                  <Typography.Text type="secondary">{t('metadataEmpty')}</Typography.Text>
                )}
              </Spin>
            </div>
          </Panel>
          <Panel header={t('info')} key="info">
            <div style={{ position: 'relative' }}>
              <FloatingActionIcon
                tooltip={t('refreshInfo')}
                icon={<ReloadOutlined />}
                onClick={handleLoadInfo}
                loading={infoLoading}
              />
              <Spin spinning={infoLoading}>
                {info ? (
                  <Typography.Paragraph className="monospace-block">{info}</Typography.Paragraph>
                ) : (
                  <Typography.Text type="secondary">{t('infoEmpty')}</Typography.Text>
                )}
              </Spin>
            </div>
          </Panel>
          <Panel header={t('folders')} key="folders">
            <div style={{ position: 'relative' }}>
              <FloatingActionIcon
                tooltip={t('refreshFolders')}
                icon={<ReloadOutlined />}
                onClick={handleLoadFolders}
                loading={foldersLoading}
              />
              <Spin spinning={foldersLoading}>
                {folders ? (
                  <Space direction="vertical" size={4}>
                    <Typography.Paragraph>
                      <Typography.Text strong>{t('storagePath')}:</Typography.Text> {folders.storagePath ?? '-'}
                    </Typography.Paragraph>
                    <Typography.Paragraph>
                      <Typography.Text strong>{t('execPath')}:</Typography.Text> {folders.execPath ?? '-'}
                    </Typography.Paragraph>
                  </Space>
                ) : (
                  <Typography.Text type="secondary">{t('foldersEmpty')}</Typography.Text>
                )}
              </Spin>
            </div>
          </Panel>
        </Collapse>
      </Space>
    </Card>
  );
}
