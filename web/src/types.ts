export interface RecordItem {
  ID: string; Platform: string; Keyword: string; Title: string; URL: string; Timestamp: number;
  Content: string; AuthorID: string; AuthorHandle: string; AuthorName: string; AuthorKey: string
}
export interface Count { Key: string; Label: string; Platform?: string; Count: number }
export interface ArchiveResponse {
  Snapshot: string; LoadedAt: string; Results: RecordItem[]; Total: number; ArchiveTotal: number; Page: number; PageSize: number;
  Keywords: Count[]; Platforms: Count[]; Authors: Count[]; Terms: Count[]; Activity: Count[];
  Coverage: { KnownAuthors: number; UnknownAuthors: number; WithContent: number; WithoutContent: number }
}
export interface Status { Source: string; State: string; Snapshot: string; LoadedAt: string; Refreshing: boolean; Stale: boolean; Error: string; RetryAt?: string; IndexPath?: string; SyncMode?: string; Phase?: string; Examined?: number; Imported?: number; Checkpoint?: number; Target?: number; LastSync?: string }
export function safeURL(value: string): string | undefined {
  try { const url = new URL(value); return ['https:', 'http:'].includes(url.protocol) ? url.href : undefined } catch { return undefined }
}
