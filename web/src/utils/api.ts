import axios from 'axios';

export interface ToolDownloadProcess {
  currentDownloadUrlIndex?: number;
  fileSize?: number;
  status?: string;
  attemptIndex?: number;
  totalAttempts?: number;
  currentUrl?: string;
  failedUrls?: string[];
  allUrls?: string[];
}

export interface ToolInfo {
  name: string;
  version: string;
  installed: boolean;
  preinstalled: boolean;
  storageFolder?: string;
  execFolder?: string;
  execFromTemp?: boolean;
  isExecutable: boolean;
  enabled: boolean;
  metadataJson?: string;
  downloadProcess?: ToolDownloadProcess;
}

export interface ToolGroup {
  name: string;
  enabled: boolean;
  tools: ToolInfo[];
}

export interface ProgressMessage {
  toolName: string;
  version?: string;
  status?: string;
  totalBytes?: number;
  downloadedBytes?: number;
  speed?: number;
  error?: string;
  attemptIndex?: number;
  totalAttempts?: number;
  currentUrl?: string;
  failedUrls?: string[];
  allUrls?: string[];
}

export interface ToolRuntimeStatus {
  name: string;
  version: string;
  installed: boolean;
  preinstalled: boolean;
  downloading: boolean;
  paused: boolean;
  downloadedBytes: number;
  totalBytes: number;
  enabled: boolean;
  downloadProcess?: ToolDownloadProcess;
}

export interface ToolFoldersResponse {
  storagePath?: string;
  execPath?: string;
}

export interface ToolInfoResponse {
  info?: string;
}

export interface MetadataResponse {
  metadata?: string;
}

export interface PlatformResponse {
  platform: string;
}

export interface ActiveTasksResponse {
  needsSSE: boolean;
}

const client = axios.create({
  baseURL: './',
  timeout: 15000
});

export async function fetchToolGroups(): Promise<ToolGroup[]> {
  const { data } = await client.get<ToolGroup[]>('/api/tools');
  return data;
}

export async function fetchRuntimeStatus(): Promise<ToolRuntimeStatus[]> {
  const { data } = await client.get<ToolRuntimeStatus[]>('/api/status');
  return data;
}

export async function fetchPlatform(): Promise<string> {
  const { data } = await client.get<PlatformResponse>('/api/platform');
  return data?.platform ?? '';
}

export async function toggleToolGroup(toolName: string, enabled: boolean): Promise<void> {
  await client.post('/api/toggle', { toolName, enabled });
}

export async function installTool(toolName: string, version: string): Promise<void> {
  await client.post('/api/install', { toolName, version });
}

export async function uninstallTool(toolName: string, version: string): Promise<void> {
  await client.post('/api/uninstall', { toolName, version });
}

export async function pauseTool(toolName: string, version: string): Promise<void> {
  await client.post('/api/pause', { toolName, version });
}

export async function fetchToolFolders(toolName: string, version: string): Promise<ToolFoldersResponse> {
  const { data } = await client.get<ToolFoldersResponse>('/api/tool-path', {
    params: { toolName, version }
  });
  return data;
}

export async function fetchToolInfoText(toolName: string, version: string): Promise<string> {
  const { data } = await client.get<ToolInfoResponse>('/api/tool-info', {
    params: { toolName, version }
  });
  return data?.info ?? '';
}

export async function fetchToolMetadata(toolName: string, version: string): Promise<string> {
  const { data } = await client.get<MetadataResponse>('/api/tool-metadata', {
    params: { toolName, version }
  });
  return data?.metadata ?? '';
}

export async function fetchActiveFlag(): Promise<boolean> {
  const { data } = await client.get<ActiveTasksResponse>('/api/active');
  return !!data?.needsSSE;
}
