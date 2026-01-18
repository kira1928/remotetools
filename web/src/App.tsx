import { useCallback, useEffect, useMemo, useState } from 'react';
import { ConfigProvider, Layout, Typography, Space, Select, Spin, Alert, Button, message, Tag } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { ToolGroup, ToolRuntimeStatus, ProgressMessage } from '@/utils/api';
import {
  fetchToolGroups,
  fetchRuntimeStatus,
  fetchPlatform,
  fetchActiveFlag,
  toggleToolGroup,
  installTool,
  uninstallTool,
  pauseTool
} from '@/utils/api';
import { buildToolKey } from '@/utils/format';
import { useLocale } from '@/hooks/useLocale';
import { useSSE } from '@/hooks/useSSE';
import { ToolGroupSection } from '@/components/ToolGroupSection';

const { Header, Content } = Layout;

type RuntimeMap = Record<string, ToolRuntimeStatus>;
type ProgressMap = Record<string, ProgressMessage>;

function toRuntimeMap(list: ToolRuntimeStatus[]): RuntimeMap {
  return list.reduce<RuntimeMap>((acc, item) => {
    const key = buildToolKey(item.name, item.version);
    acc[key] = item;
    return acc;
  }, {});
}

function buildProgressKey(toolName: string, version: string, groups: ToolGroup[]): string {
  if (version) {
    return buildToolKey(toolName, version);
  }
  for (const group of groups) {
    const matched = group.tools.find((tool) => tool.name === toolName);
    if (matched) {
      return buildToolKey(matched.name, matched.version);
    }
  }
  return buildToolKey(toolName, '');
}

export default function App() {
  const { t, lang, setLang, antdLocale } = useLocale();
  const [toolGroups, setToolGroups] = useState<ToolGroup[]>([]);
  const [runtimeMap, setRuntimeMap] = useState<RuntimeMap>({});
  const [progressMap, setProgressMap] = useState<ProgressMap>({});
  const [platform, setPlatform] = useState<string>('');
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [activeSSE, setActiveSSE] = useState<boolean>(false);
  const [messageApi, contextHolder] = message.useMessage();

  const fetchGroups = useCallback(async () => {
    const groups = await fetchToolGroups();
    setToolGroups(groups);
    return groups;
  }, []);

  const fetchRuntime = useCallback(async () => {
    const runtime = await fetchRuntimeStatus();
    setRuntimeMap(toRuntimeMap(runtime));
    return runtime;
  }, []);

  const refreshActiveFlag = useCallback(async () => {
    try {
      const active = await fetchActiveFlag();
      setActiveSSE(active);
    } catch (err) {
      console.error('fetch active flag failed', err);
    }
  }, []);

  const loadInitial = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [groups] = await Promise.all([
        fetchGroups(),
        fetchRuntime(),
        fetchPlatform().then(setPlatform),
        refreshActiveFlag()
      ]);
      if (groups.length === 0) {
        setProgressMap({});
      }
    } catch (err) {
      console.error('initial load failed', err);
      setError(t('failedToLoad'));
      messageApi.error(t('failedToLoad'));
    } finally {
      setLoading(false);
    }
  }, [fetchGroups, fetchRuntime, refreshActiveFlag, t, messageApi]);

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  const handleInstall = useCallback(
    async (toolName: string, version: string) => {
      try {
        await installTool(toolName, version);
        messageApi.success(t('installing'));
        await Promise.all([fetchGroups(), fetchRuntime(), refreshActiveFlag()]);
      } catch (err) {
        console.error('install failed', err);
        messageApi.error(t('installFailed'));
      }
    },
    [fetchGroups, fetchRuntime, refreshActiveFlag, messageApi, t]
  );

  const handlePause = useCallback(
    async (toolName: string, version: string) => {
      try {
        await pauseTool(toolName, version);
        await Promise.all([fetchGroups(), fetchRuntime(), refreshActiveFlag()]);
      } catch (err) {
        console.error('pause failed', err);
        messageApi.error(t('pauseFailed'));
      }
    },
    [fetchGroups, fetchRuntime, refreshActiveFlag, messageApi, t]
  );

  const handleResume = useCallback(
    async (toolName: string, version: string) => {
      try {
        await installTool(toolName, version);
        await Promise.all([fetchGroups(), fetchRuntime(), refreshActiveFlag()]);
      } catch (err) {
        console.error('resume failed', err);
        messageApi.error(t('resumeFailed'));
      }
    },
    [fetchGroups, fetchRuntime, refreshActiveFlag, messageApi, t]
  );

  const handleUninstall = useCallback(
    async (toolName: string, version: string) => {
      try {
        await uninstallTool(toolName, version);
        messageApi.success(t('uninstallCompleted'));
        await Promise.all([fetchGroups(), fetchRuntime(), refreshActiveFlag()]);
      } catch (err) {
        console.error('uninstall failed', err);
        messageApi.error(t('uninstallFailedAction'));
      }
    },
    [fetchGroups, fetchRuntime, refreshActiveFlag, messageApi, t]
  );

  const handleToggleGroup = useCallback(
    async (groupName: string, enabled: boolean) => {
      try {
        await toggleToolGroup(groupName, enabled);
        await Promise.all([fetchGroups(), fetchRuntime(), refreshActiveFlag()]);
      } catch (err) {
        console.error('toggle group failed', err);
        messageApi.error(t('groupToggleFailed'));
      }
    },
    [fetchGroups, fetchRuntime, refreshActiveFlag, messageApi, t]
  );

  const handleProgressMessage = useCallback(
    async (event: MessageEvent<any>) => {
      try {
        const payload: ProgressMessage = JSON.parse(event.data);
        if (!payload?.toolName) {
          return;
        }
        const key = buildProgressKey(payload.toolName, payload.version ?? '', toolGroups);
        const status = payload.status ?? '';
        setProgressMap((prev) => {
          const next = { ...prev };
          if (['completed', 'failed', 'uninstalled'].includes(status)) {
            delete next[key];
          } else {
            next[key] = payload;
          }
          return next;
        });
        if (['completed', 'failed', 'uninstalled'].includes(status)) {
          await Promise.all([fetchGroups(), fetchRuntime(), refreshActiveFlag()]);
        }
      } catch (err) {
        console.error('parse progress message failed', err);
      }
    },
    [fetchGroups, fetchRuntime, refreshActiveFlag, toolGroups]
  );

  useSSE('./api/progress', handleProgressMessage, {
    enabled: activeSSE,
    onError: (event) => {
      console.error('sse error', event);
      setActiveSSE(false);
    }
  });

  const languageOptions = useMemo(
    () => [
      { label: '中文', value: 'zh' },
      { label: 'English', value: 'en' }
    ],
    []
  );

  return (
    <ConfigProvider locale={antdLocale}>
      {contextHolder}
      <Layout style={{ minHeight: '100vh' }}>
        <Header style={{ background: '#fff', padding: '0 24px' }}>
          <Space style={{ width: '100%', justifyContent: 'space-between', alignItems: 'center' }}>
            <Space size={12} align="center" wrap>
              <Typography.Title level={3} style={{ margin: 0 }}>
                {t('title')}
              </Typography.Title>
              {platform && <Tag color="geekblue">{`${t('platform')}: ${platform}`}</Tag>}
              {activeSSE && <Tag color="green">{t('activeTasks')}</Tag>}
            </Space>
            <Space size={12} align="center" wrap>
              <Select
                value={lang}
                options={languageOptions}
                onChange={(value) => setLang(value as 'zh' | 'en')}
                style={{ width: 120 }}
              />
              <Button icon={<ReloadOutlined />} onClick={() => loadInitial()}>
                {t('refresh')}
              </Button>
            </Space>
          </Space>
        </Header>
        <Content style={{ padding: '24px', background: '#f5f5f5' }}>
          {loading ? (
            <Space direction="vertical" style={{ width: '100%', alignItems: 'center', marginTop: 120 }}>
              <Spin size="large" tip={t('loading')} />
            </Space>
          ) : error ? (
            <Alert type="error" message={error} showIcon />
          ) : (
            <div className="group-grid">
              {toolGroups.map((group) => (
                <ToolGroupSection
                  key={group.name}
                  group={group}
                  runtimeMap={runtimeMap}
                  progressMap={progressMap}
                  onToggleGroup={handleToggleGroup}
                  onInstall={handleInstall}
                  onPause={handlePause}
                  onResume={handleResume}
                  onUninstall={handleUninstall}
                  messageApi={messageApi}
                />
              ))}
            </div>
          )}
        </Content>
      </Layout>
    </ConfigProvider>
  );
}
