import type { ToolInfo, ToolRuntimeStatus, ProgressMessage, ToolDownloadProcess } from '@/utils/api';
import { formatEta, formatBytes } from '@/utils/format';

export interface ResolvedProgress {
  downloadedBytes: number;
  totalBytes: number;
  percent: number;
  speed?: number;
  status?: string;
  attemptIndex?: number;
  totalAttempts?: number;
  currentUrl?: string;
  failedUrls: string[];
  allUrls: string[];
  eta: string | null;
}

export function resolveProgress(tool: ToolInfo, runtime?: ToolRuntimeStatus, progress?: ProgressMessage): ResolvedProgress {
  const downloadProcess: ToolDownloadProcess | undefined = progress?.status
    ? { ...tool.downloadProcess, status: progress.status }
    : tool.downloadProcess ?? runtime?.downloadProcess;
  const downloadedBytes = progress?.downloadedBytes ?? runtime?.downloadedBytes ?? 0;
  const totalBytes = progress?.totalBytes ?? runtime?.totalBytes ?? downloadProcess?.fileSize ?? 0;
  const percent = totalBytes > 0 ? Math.min(100, Number(((downloadedBytes / totalBytes) * 100).toFixed(1))) : 0;
  const speed = progress?.speed;
  const status = progress?.status ?? downloadProcess?.status ?? (runtime?.downloading ? 'downloading' : undefined);
  const attemptIndex = progress?.attemptIndex ?? downloadProcess?.attemptIndex;
  const totalAttempts = progress?.totalAttempts ?? downloadProcess?.totalAttempts;
  const currentUrl = progress?.currentUrl ?? downloadProcess?.currentUrl;
  const failedUrls = progress?.failedUrls ?? downloadProcess?.failedUrls ?? [];
  const allUrls = progress?.allUrls ?? downloadProcess?.allUrls ?? [];

  let eta: string | null = null;
  if (speed && speed > 0 && totalBytes > 0 && downloadedBytes >= 0) {
    const remaining = totalBytes - downloadedBytes;
    eta = remaining > 0 ? formatEta(remaining / speed) : formatEta(0);
  }

  return {
    downloadedBytes,
    totalBytes,
    percent,
    speed,
    status,
    attemptIndex,
    totalAttempts,
    currentUrl,
    failedUrls,
    allUrls,
    eta
  };
}

export type ToolStatusKey =
  | 'downloading'
  | 'trying'
  | 'extracting'
  | 'paused'
  | 'completed'
  | 'failed'
  | 'notInstalled';

const activeStatuses: ToolStatusKey[] = ['downloading', 'trying', 'extracting'];
const resumableStatuses: ToolStatusKey[] = ['paused'];
const installableStatuses: ToolStatusKey[] = ['notInstalled', 'failed'];

export function deriveToolStatus(tool: ToolInfo, runtime?: ToolRuntimeStatus, progress?: ProgressMessage): ToolStatusKey {
  const resolved = resolveProgress(tool, runtime, progress);
  const rawStatus = resolved.status ?? (runtime?.paused ? 'paused' : undefined);
  const status = normalizeStatus(rawStatus);

  if (status) {
    if (status === 'completed' && !tool.installed && !runtime?.installed) {
      return 'notInstalled';
    }
    return status;
  }

  if (runtime?.downloading) {
    return 'downloading';
  }

  if (runtime?.paused) {
    return 'paused';
  }

  if (tool.installed || runtime?.installed) {
    return 'completed';
  }

  return 'notInstalled';
}

export function hasActiveDownload(status: ToolStatusKey): boolean {
  return activeStatuses.includes(status);
}

export function isResumableStatus(status: ToolStatusKey): boolean {
  return resumableStatuses.includes(status);
}

export function formatProgressSummary(resolved: ResolvedProgress): string {
  const { downloadedBytes, totalBytes } = resolved;
  if (!totalBytes) {
    if (!downloadedBytes) {
      return '0%';
    }
    return formatBytes(downloadedBytes);
  }
  return `${formatBytes(downloadedBytes)} / ${formatBytes(totalBytes)}`;
}

export function isInstallableStatus(status: ToolStatusKey): boolean {
  return installableStatuses.includes(status);
}

export function isInstalledStatus(status: ToolStatusKey): boolean {
  return status === 'completed';
}

function normalizeStatus(status?: string): ToolStatusKey | undefined {
  if (!status) {
    return undefined;
  }
  if (status === 'failed' || status === 'paused' || status === 'downloading' || status === 'trying' || status === 'extracting' || status === 'completed') {
    return status;
  }
  if (status === 'notInstalled') {
    return 'notInstalled';
  }
  if (status === 'uninstalled') {
    return 'notInstalled';
  }
  return status as ToolStatusKey;
}
