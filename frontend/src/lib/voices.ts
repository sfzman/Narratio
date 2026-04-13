import type {VoicesResponse} from '@/types/api';
import {apiRequest} from './api';

export async function listVoices(): Promise<VoicesResponse> {
  return apiRequest<VoicesResponse>('/voices');
}
