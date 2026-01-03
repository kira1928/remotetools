import { createContext, useContext, useMemo, useState, useEffect, ReactNode } from 'react';
import zhCN from 'antd/locale/zh_CN';
import enUS from 'antd/locale/en_US';
import type { Locale as AntdLocale } from 'antd/es/locale';

const STORAGE_KEY = 'remotetools-lang';

type SupportedLang = 'zh' | 'en';

type TranslationDict = Record<string, string>;

type TranslationMap = Record<SupportedLang, TranslationDict>;

const translations: TranslationMap = {
  zh: {
    title: '外部工具管理器',
    platform: '运行平台',
    loading: '加载工具中...',
    failedToLoad: '加载工具失败',
    installed: '已安装',
    preinstalled: '预装',
    notInstalled: '未安装',
    installing: '安装中',
    uninstalling: '卸载中',
    downloading: '下载中',
    trying: '尝试中',
    extracting: '解压中',
    paused: '已暂停',
    completed: '已完成',
    failed: '失败',
    uninstallFailed: '卸载失败',
    uninstallCompleted: '卸载完成',
    enabled: '已启用',
    disabled: '已禁用',
    enableGroup: '启用工具组',
  defaultVersion: '默认版本',
  defaultVersionTag: '默认版本',
  noVersionAvailable: '无可用版本',
    install: '安装',
    reinstall: '重新安装',
    pause: '暂停',
    resume: '继续',
    uninstall: '卸载',
    notUninstallable: '不可卸载',
    metadata: '元数据',
  refreshMetadata: '刷新元数据',
    info: '工具信息',
  refreshInfo: '刷新工具信息',
  infoEmpty: '暂无工具信息',
    folders: '目录',
  refreshFolders: '刷新目录信息',
  foldersEmpty: '暂无目录信息',
    storagePath: '存储路径',
    execPath: '执行路径',
    execFromTemp: '临时目录运行',
    nonExecutable: '非可执行程序',
    showMetadata: '加载元数据',
    showInfo: '加载信息',
    showFolders: '加载目录',
    metadataEmpty: '暂无元数据信息',
    copyMetadata: '复制元数据',
    copySuccess: '元数据已复制',
    copyFailed: '复制失败',
  metadataParseFailed: '无法解析为合法的 JSON',
    eta: '预计剩余时间',
    etaCalculating: '计算中...',
    etaDone: '已完成',
    downloadBytes: '下载进度',
    speed: '当前速度',
    mirrorStatusTrying: '尝试中',
    mirrorStatusDownloading: '下载中',
    mirrorStatusFailed: '失败',
    mirrorStatusExtracting: '解压中',
    mirrorStatusSuccess: '完成',
    activeTasks: '活跃任务',
  installedVersions: '已安装版本',
  noInstalledVersion: '尚未安装任何版本',
  downloadingVersions: '下载中的版本',
  noActiveDownloads: '暂无下载任务',
  pausedVersions: '暂停中的版本',
  noPausedDownloads: '暂无暂停的下载',
  pendingVersions: '待下载版本',
  noPendingVersions: '暂无待下载版本',
  noToolsInGroup: '该工具组暂无工具',
    toolDisabled: '该工具组已禁用，启用后方可继续操作。',
    refresh: '刷新',
    language: '界面语言',
    groupToggleFailed: '切换工具组失败',
  groupPauseSuccess: '已暂停当前组的下载任务',
  groupPauseFailed: '暂停下载失败',
  groupResumeSuccess: '已继续当前组的下载任务',
  groupResumeFailed: '继续下载失败',
  groupPauseAll: '暂停全部下载',
  groupResumeAll: '继续全部下载',
  groupInstallAll: '下载全部版本',
  groupInstallQueued: '已开始下载该组全部版本',
  groupInstallFailed: '启动下载失败',
    installFailed: '安装失败',
    pauseFailed: '暂停失败',
    resumeFailed: '继续失败',
    uninstallFailedAction: '卸载失败',
    metadataFailed: '加载元数据失败',
    infoFailed: '加载信息失败',
    folderFailed: '加载目录失败'
    ,
    viewDetails: '展开详情',
    hideDetails: '收起详情'
  },
  en: {
    title: 'External Tools Manager',
    platform: 'Platform',
    loading: 'Loading tools...',
    failedToLoad: 'Failed to load tools',
    installed: 'Installed',
    preinstalled: 'Preinstalled',
    notInstalled: 'Not Installed',
    installing: 'Installing',
    uninstalling: 'Uninstalling',
    downloading: 'Downloading',
    trying: 'Trying',
    extracting: 'Extracting',
    paused: 'Paused',
    completed: 'Completed',
    failed: 'Failed',
    uninstallFailed: 'Uninstall Failed',
    uninstallCompleted: 'Uninstall Completed',
    enabled: 'Enabled',
    disabled: 'Disabled',
    enableGroup: 'Enable Tool Group',
  defaultVersion: 'Default Version',
  defaultVersionTag: 'Default',
  noVersionAvailable: 'No version available',
    install: 'Install',
    reinstall: 'Reinstall',
    pause: 'Pause',
    resume: 'Resume',
    uninstall: 'Uninstall',
    notUninstallable: 'Cannot Uninstall',
    metadata: 'Metadata',
  refreshMetadata: 'Refresh metadata',
    info: 'Info',
  refreshInfo: 'Refresh info',
  infoEmpty: 'No additional info',
    folders: 'Folders',
  refreshFolders: 'Refresh folders',
  foldersEmpty: 'No folder information',
    storagePath: 'Storage Path',
    execPath: 'Exec Path',
    execFromTemp: 'Running from temp directory',
    nonExecutable: 'Not executable',
    showMetadata: 'Load metadata',
    showInfo: 'Load info',
    showFolders: 'Load folders',
    metadataEmpty: 'No metadata available',
    copyMetadata: 'Copy metadata',
    copySuccess: 'Metadata copied',
    copyFailed: 'Copy failed',
  metadataParseFailed: 'Metadata is not valid JSON',
    eta: 'Remaining time',
    etaCalculating: 'Calculating...',
    etaDone: 'Done',
    downloadBytes: 'Progress',
    speed: 'Speed',
    mirrorStatusTrying: 'Trying',
    mirrorStatusDownloading: 'Downloading',
    mirrorStatusFailed: 'Failed',
    mirrorStatusExtracting: 'Extracting',
    mirrorStatusSuccess: 'Succeeded',
    activeTasks: 'Active Tasks',
  installedVersions: 'Installed Versions',
  noInstalledVersion: 'No versions installed',
  downloadingVersions: 'Downloading Versions',
  noActiveDownloads: 'No active downloads',
  pausedVersions: 'Paused Versions',
  noPausedDownloads: 'No paused downloads',
  pendingVersions: 'Pending Versions',
  noPendingVersions: 'No pending versions',
  noToolsInGroup: 'No tools in this group',
    toolDisabled: 'Tool group disabled. Enable to continue.',
    refresh: 'Refresh',
    language: 'Language',
    groupToggleFailed: 'Failed to toggle group',
  groupPauseSuccess: 'Paused downloads for this group',
  groupPauseFailed: 'Failed to pause downloads',
  groupResumeSuccess: 'Resumed downloads for this group',
  groupResumeFailed: 'Failed to resume downloads',
  groupPauseAll: 'Pause all downloads',
  groupResumeAll: 'Resume all downloads',
  groupInstallAll: 'Download all versions',
  groupInstallQueued: 'Started downloading all versions in this group',
  groupInstallFailed: 'Failed to start batch download',
    installFailed: 'Install failed',
    pauseFailed: 'Pause failed',
    resumeFailed: 'Resume failed',
    uninstallFailedAction: 'Uninstall failed',
    metadataFailed: 'Failed to load metadata',
    infoFailed: 'Failed to load info',
    folderFailed: 'Failed to load folders',
    viewDetails: 'View details',
    hideDetails: 'Hide details'
  }
};

interface LocaleContextValue {
  lang: SupportedLang;
  setLang: (lang: SupportedLang) => void;
  t: (key: keyof TranslationDict | string) => string;
  antdLocale: AntdLocale;
}

const LocaleContext = createContext<LocaleContextValue | null>(null);

export function LocaleProvider({ defaultLang = 'zh', children }: { defaultLang?: SupportedLang; children: ReactNode }) {
  const initialLang: SupportedLang = (() => {
    const stored = typeof window !== 'undefined' ? (localStorage.getItem(STORAGE_KEY) as SupportedLang | null) : null;
    if (stored === 'zh' || stored === 'en') {
      return stored;
    }
    if (typeof navigator !== 'undefined') {
      const nav = navigator as Navigator & { userLanguage?: string };
      const navLang = nav.language || nav.userLanguage || '';
      if (navLang.toLowerCase().startsWith('zh')) {
        return 'zh';
      }
    }
    return defaultLang;
  })();

  const [lang, setLangState] = useState<SupportedLang>(initialLang);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, lang);
    }
  }, [lang]);

  const value = useMemo<LocaleContextValue>(() => {
    const dict = translations[lang];
    const t = (key: string) => dict[key] ?? key;
    const antdLocale = lang === 'zh' ? zhCN : enUS;
    return {
      lang,
      setLang: setLangState,
      t,
      antdLocale
    };
  }, [lang]);

  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

export function useLocaleContext(): LocaleContextValue {
  const ctx = useContext(LocaleContext);
  if (!ctx) {
    throw new Error('useLocaleContext must be used within LocaleProvider');
  }
  return ctx;
}
