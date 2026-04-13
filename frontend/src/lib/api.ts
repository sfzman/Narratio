import type {ApiErrorResponse, ApiSuccessResponse} from '@/types/api';

const DEFAULT_API_BASE_URL = '/api/v1';

function normalizeBaseUrl(baseUrl: string): string {
  return baseUrl.replace(/\/$/, '');
}

export function getApiBaseUrl(): string {
  const value = import.meta.env.VITE_API_BASE_URL;
  if (typeof value === 'string' && value.trim() !== '') {
    return normalizeBaseUrl(value.trim());
  }
  return DEFAULT_API_BASE_URL;
}

export class ApiError extends Error {
  code?: number;
  requestId?: string;
  httpStatus?: number;

  constructor(message: string, options?: {code?: number; requestId?: string; httpStatus?: number}) {
    super(message);
    this.name = 'ApiError';
    this.code = options?.code;
    this.requestId = options?.requestId;
    this.httpStatus = options?.httpStatus;
  }
}

function buildUrl(path: string): string {
  if (/^https?:\/\//.test(path)) {
    return path;
  }
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;
  return `${getApiBaseUrl()}${normalizedPath}`;
}

function isApiSuccessResponse<T>(value: unknown): value is ApiSuccessResponse<T> {
  return Boolean(value) && typeof value === 'object' && 'code' in value && 'data' in value;
}

function isApiErrorResponse(value: unknown): value is ApiErrorResponse {
  return Boolean(value) && typeof value === 'object' && 'code' in value && 'message' in value;
}

export async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers ?? {});
  if (init?.body != null && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(buildUrl(path), {
    ...init,
    headers,
  });

  const text = await response.text();
  const payload = text ? JSON.parse(text) : null;

  if (!response.ok) {
    if (isApiErrorResponse(payload)) {
      throw new ApiError(payload.message, {
        code: payload.code,
        requestId: payload.request_id,
        httpStatus: response.status,
      });
    }
    throw new ApiError(`HTTP ${response.status}`, {httpStatus: response.status});
  }

  if (!isApiSuccessResponse<T>(payload)) {
    throw new ApiError('Unexpected API response shape', {httpStatus: response.status});
  }

  return payload.data;
}
