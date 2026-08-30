/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'
import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ModelOption,
  GroupOption,
} from './types'

export function isVideoGenerationModel(model: string): boolean {
  const normalized = model.trim().toLowerCase()
  return (
    normalized.includes('sora2') ||
    normalized.includes('sora-2') ||
    normalized.includes('video-2.0') ||
    normalized.includes('video-2.5') ||
    ['wan3.0-480p', 'wan3.0-720p', 'wan3.0-1080p'].includes(normalized) ||
    normalized.includes('ko3') ||
    [
      'minimax-h3-480p',
      'minimax-h3-768p',
      'minimax-h3-2k',
      'minimax-h3-4k',
    ].includes(normalized)
  )
}

function getMiniMaxH3Size(model: string): string {
  switch (model.toLowerCase()) {
    case 'minimax-h3-480p':
      return '856x480'
    case 'minimax-h3-768p':
      return '1376x768'
    case 'minimax-h3-4k':
      return '3840x2160'
    default:
      return '2560x1440'
  }
}

function extractVideoPrompt(payload: ChatCompletionRequest): string {
  const userMessages = payload.messages.filter((message) => message.role === 'user')
  const lastUserMessage = userMessages[userMessages.length - 1]
  if (!lastUserMessage) return 'Generate a short cinematic video.'

  if (typeof lastUserMessage.content === 'string') {
    return lastUserMessage.content
  }

  const textParts = lastUserMessage.content
    .filter((part) => part.type === 'text' && part.text)
    .map((part) => part.text)

  return textParts.join('\n') || 'Generate a short cinematic video.'
}

function extractVideoImageURLs(payload: ChatCompletionRequest): string[] {
  const userMessages = payload.messages.filter((message) => message.role === 'user')
  const urls: string[] = []
  for (const message of userMessages) {
    if (typeof message.content === 'string') continue
    for (const part of message.content) {
      if (part.type === 'image_url' && part.image_url?.url) {
        urls.push(part.image_url.url)
      }
    }
  }
  return [...new Set(urls)]
}

export async function sendVideoGeneration(
  payload: ChatCompletionRequest
): Promise<Record<string, unknown>> {
  const model = payload.model.trim().toLowerCase()
  const isMiniMaxH3 = [
    'minimax-h3-480p',
    'minimax-h3-768p',
    'minimax-h3-2k',
    'minimax-h3-4k',
  ].includes(model)
  const isVideo25480p = model === 'video-2.5-480p'
  const isWan30 = ['wan3.0-480p', 'wan3.0-720p', 'wan3.0-1080p'].includes(model)
  const imageURLs = extractVideoImageURLs(payload)
  const videoBody: Record<string, unknown> = {
    model: payload.model,
    group: payload.group,
    prompt: extractVideoPrompt(payload),
    seconds: String(payload.duration || (isMiniMaxH3 || isWan30 ? 5 : 4)),
    size: isMiniMaxH3
      ? getMiniMaxH3Size(model)
      : isVideo25480p
        ? '864x496'
        : model === 'wan3.0-480p'
          ? '854x480'
          : model === 'wan3.0-1080p'
            ? '1920x1080'
            : '1280x720',
  }
  if (imageURLs.length === 1) {
    videoBody.image_url = imageURLs[0]
    videoBody.input_reference = imageURLs[0]
  } else if (imageURLs.length > 1) {
    videoBody.image_urls = imageURLs
  }
  const res = await api.post(
    API_ENDPOINTS.VIDEO_GENERATIONS,
    videoBody,
    {
      skipErrorHandler: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS)
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.map((model: string) => ({
    label: model,
    value: model,
  }))
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}
