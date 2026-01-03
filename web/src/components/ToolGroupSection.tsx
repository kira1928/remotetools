import { useState, useMemo, useCallback } from 'react';
import type { ReactNode } from 'react';
import { Card, Space, Switch, Typography, Button, Tag, Tooltip, Divider } from 'antd';
import { PauseCircleOutlined, PlayCircleOutlined, DownloadOutlined, DownOutlined, UpOutlined } from '@ant-design/icons';
import type { MessageInstance } from 'antd/es/message/interface';
import type { ToolGroup, ToolRuntimeStatus, ProgressMessage, ToolInfo } from '@/utils/api';
import { buildToolKey } from '@/utils/format';
import { ToolCard } from '@/components/ToolCard';
import { useLocale } from '@/hooks/useLocale';
import {
  resolveProgress,
  deriveToolStatus,
  hasActiveDownload,
  isResumableStatus,
  isInstalledStatus,
  formatProgressSummary
} from '@/utils/toolStatus';
import type { ResolvedProgress, ToolStatusKey } from '@/utils/toolStatus';

interface ToolGroupSectionProps {
  group: ToolGroup;
  runtimeMap: Record<string, ToolRuntimeStatus>;
  progressMap: Record<string, ProgressMessage>;
  onToggleGroup: (groupName: string, enabled: boolean) => Promise<void>;
  onInstall: (toolName: string, version: string) => Promise<void>;
  onPause: (toolName: string, version: string) => Promise<void>;
  onResume: (toolName: string, version: string) => Promise<void>;
  onUninstall: (toolName: string, version: string) => Promise<void>;
  messageApi: MessageInstance;
}

interface ToolStateView {
  key: string;
  tool: ToolInfo;
  runtime?: ToolRuntimeStatus;
  progress?: ProgressMessage;
  resolved: ResolvedProgress;
  status: ToolStatusKey;
}

export function ToolGroupSection({
  group,
  runtimeMap,
  progressMap,
  onToggleGroup,
  onInstall,
  onPause,
  onResume,
  onUninstall,
  messageApi
}: ToolGroupSectionProps) {
  const { t } = useLocale();
  const [toggleLoading, setToggleLoading] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const toolStates = useMemo<ToolStateView[]>(() => {
    return group.tools.map((tool) => {
      const key = buildToolKey(tool.name, tool.version);
      const runtime = runtimeMap[key];
      const progress = progressMap[key];
      const resolved = resolveProgress(tool, runtime, progress);
      const status = deriveToolStatus(tool, runtime, progress);
      return { key, tool, runtime, progress, resolved, status };
    });
  }, [group.tools, progressMap, runtimeMap]);

  const hasTools = toolStates.length > 0;

  const defaultState = useMemo<ToolStateView | null>(() => {
    if (!toolStates.length) {
      return null;
    }
    const enabledState = toolStates.find((state) => state.tool.enabled);
    return enabledState ?? toolStates[0];
  }, [toolStates]);

  const installedStates = useMemo(() => toolStates.filter((state) => isInstalledStatus(state.status)), [toolStates]);
  const activeDownloads = useMemo(() => toolStates.filter((state) => hasActiveDownload(state.status)), [toolStates]);
  const pausedStates = useMemo(() => toolStates.filter((state) => isResumableStatus(state.status)), [toolStates]);
  const pendingStates = useMemo(
    () =>
      toolStates.filter(
        (state) => state.status === 'notInstalled' || state.status === 'failed'
      ),
    [toolStates]
  );

  const handleToggle = useCallback(
    async (checked: boolean) => {
      setToggleLoading(true);
      try {
        await onToggleGroup(group.name, checked);
      } finally {
        setToggleLoading(false);
      }
    },
    [group.name, onToggleGroup]
  );

  const handleGroupPause = useCallback(async () => {
    if (!activeDownloads.length) {
      return;
    }
    try {
      await Promise.all(activeDownloads.map((state) => onPause(state.tool.name, state.tool.version)));
      messageApi.success(t('groupPauseSuccess'));
    } catch (error) {
      console.error('group pause failed', error);
      messageApi.error(t('groupPauseFailed'));
    }
  }, [activeDownloads, messageApi, onPause, t]);

  const handleGroupResume = useCallback(async () => {
    if (!pausedStates.length) {
      return;
    }
    try {
      await Promise.all(pausedStates.map((state) => onResume(state.tool.name, state.tool.version)));
      messageApi.success(t('groupResumeSuccess'));
    } catch (error) {
      console.error('group resume failed', error);
      messageApi.error(t('groupResumeFailed'));
    }
  }, [messageApi, onResume, pausedStates, t]);

  const toggleExpand = useCallback(() => {
    setExpanded((prev) => !prev);
  }, []);

  const handleGroupInstall = useCallback(async () => {
    if (!pendingStates.length) {
      return;
    }
    try {
      await Promise.all(pendingStates.map((state) => onInstall(state.tool.name, state.tool.version)));
      messageApi.success(t('groupInstallQueued'));
    } catch (error) {
      console.error('group install failed', error);
      messageApi.error(t('groupInstallFailed'));
    }
  }, [messageApi, onInstall, pendingStates, t]);

  const defaultInstalled = defaultState ? isInstalledStatus(defaultState.status) : false;

  let groupAction: ReactNode = null;
  if (activeDownloads.length > 0) {
    groupAction = (
      <Tooltip title={t('groupPauseAll')}>
        <Button icon={<PauseCircleOutlined />} onClick={handleGroupPause} />
      </Tooltip>
    );
  } else if (pausedStates.length > 0) {
    groupAction = (
      <Tooltip title={t('groupResumeAll')}>
        <Button icon={<PlayCircleOutlined />} onClick={handleGroupResume} />
      </Tooltip>
    );
  } else if (pendingStates.length > 0) {
    groupAction = (
      <Tooltip title={t('groupInstallAll')}>
        <Button icon={<DownloadOutlined />} onClick={handleGroupInstall} />
      </Tooltip>
    );
  }

  return (
    <Card
      title={
        <Typography.Title level={4} style={{ margin: 0 }}>
          {group.name}
        </Typography.Title>
      }
      extra={
        <Space size={8}>
          {groupAction}
          <Switch checked={group.enabled} loading={toggleLoading} onChange={handleToggle} />
        </Space>
      }
      bordered
    >
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Space size={8} wrap>
          <Tag color={group.enabled ? 'green' : 'default'}>
            {group.enabled ? t('enabled') : t('disabled')}
          </Tag>
          {defaultState?.tool.enabled && <Tag color="blue">{t('defaultVersionTag')}</Tag>}
        </Space>

        {hasTools ? (
          <Typography.Paragraph style={{ marginBottom: 0 }}>
            <Typography.Text strong>{t('defaultVersion')}:</Typography.Text>{' '}
            <Tag color="geekblue" style={{ marginRight: 8 }}>
              {defaultState ? defaultState.tool.version : t('noVersionAvailable')}
            </Tag>
            <Typography.Text type={defaultInstalled ? 'success' : 'secondary'}>
              {defaultInstalled ? t('installed') : t('notInstalled')}
            </Typography.Text>
          </Typography.Paragraph>
        ) : (
          <Typography.Text type="secondary">{t('noToolsInGroup')}</Typography.Text>
        )}

        {hasTools && (
          <Button type="link" onClick={toggleExpand} icon={expanded ? <UpOutlined /> : <DownOutlined />}>
            {expanded ? t('hideDetails') : t('viewDetails')}
          </Button>
        )}

        {expanded && hasTools && (
          <>
            <Divider style={{ margin: '8px 0' }} />
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              {installedStates.length > 0 && (
                <section>
                  <Typography.Text strong>{t('installedVersions')}:</Typography.Text>
                  <Space size={8} wrap style={{ marginTop: 4 }}>
                    {installedStates.map((state) => (
                      <Tag color={state.tool.preinstalled ? 'gold' : 'green'} key={`installed-${state.key}`}>
                        {state.tool.version}
                      </Tag>
                    ))}
                  </Space>
                </section>
              )}

              {activeDownloads.length > 0 && (
                <section>
                  <Typography.Text strong>{t('downloadingVersions')}:</Typography.Text>
                  <Space size={8} wrap style={{ marginTop: 4 }}>
                    {activeDownloads.map((state) => (
                      <Tag color="blue" key={`active-${state.key}`}>
                        {state.tool.version} · {Math.round(state.resolved.percent)}% · {formatProgressSummary(state.resolved)}
                      </Tag>
                    ))}
                  </Space>
                </section>
              )}

              {pausedStates.length > 0 && (
                <section>
                  <Typography.Text strong>{t('pausedVersions')}:</Typography.Text>
                  <Space size={8} wrap style={{ marginTop: 4 }}>
                    {pausedStates.map((state) => (
                      <Tag color="purple" key={`paused-${state.key}`}>
                        {state.tool.version} · {formatProgressSummary(state.resolved)}
                      </Tag>
                    ))}
                  </Space>
                </section>
              )}

              {pendingStates.length > 0 && (
                <section>
                  <Typography.Text strong>{t('pendingVersions')}:</Typography.Text>
                  <Space size={8} wrap style={{ marginTop: 4 }}>
                    {pendingStates.map((state) => (
                      <Tag color="default" key={`pending-${state.key}`}>
                        {state.tool.version}
                      </Tag>
                    ))}
                  </Space>
                </section>
              )}
            </Space>
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
              {toolStates.map((state) => (
                <ToolCard
                  key={state.key}
                  tool={state.tool}
                  runtime={state.runtime}
                  progress={state.progress}
                  resolved={state.resolved}
                  status={state.status}
                  onInstall={onInstall}
                  onPause={onPause}
                  onResume={onResume}
                  onUninstall={onUninstall}
                  messageApi={messageApi}
                />
              ))}
            </Space>
          </>
        )}
      </Space>
    </Card>
  );
}
