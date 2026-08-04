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
  const normalized = model.toLowerCase()
  return (
    normalized.includes('sora2') ||
    normalized.includes('sora-2') ||
    normalized.includes('video-2.0') ||
    normalized.includes('ko3') ||
    normalized.includes('minimax-h3')
  )
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

export async function sendVideoGeneration(
  payload: ChatCompletionRequest
): Promise<Record<string, unknown>> {
  const isMiniMaxH3 = payload.model.toLowerCase() === 'minimax-h3'
  const res = await api.post(
    API_ENDPOINTS.VIDEO_GENERATIONS,
    {
      model: payload.model,
      group: payload.group,
      prompt: extractVideoPrompt(payload),
      seconds: isMiniMaxH3 ? '5' : '4',
      size: isMiniMaxH3 ? '2560x1440' : '1280x720',
    },
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
