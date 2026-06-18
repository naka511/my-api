/*
Copyright (C) 2025 QuantumNous

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

export const MESSAGE_STATUS = {
  LOADING: 'loading',
  INCOMPLETE: 'incomplete',
  COMPLETE: 'complete',
  ERROR: 'error',
};

export const MESSAGE_ROLES = {
  USER: 'user',
  ASSISTANT: 'assistant',
  SYSTEM: 'system',
};

// 默认消息示例 - 使用函数生成以支持 i18n
export const getDefaultMessages = (t) => [
  {
    role: MESSAGE_ROLES.USER,
    id: '2',
    createAt: 1715676751919,
    content: t('默认用户消息'),
  },
  {
    role: MESSAGE_ROLES.ASSISTANT,
    id: '3',
    createAt: 1715676751919,
    content: t('默认助手消息'),
    reasoningContent: '',
    isReasoningExpanded: false,
  },
];

// 保留旧的导出以保持向后兼容
export const DEFAULT_MESSAGES = [
  {
    role: MESSAGE_ROLES.USER,
    id: '2',
    createAt: 1715676751919,
    content: 'Hello',
  },
  {
    role: MESSAGE_ROLES.ASSISTANT,
    id: '3',
    createAt: 1715676751919,
    content: 'Hello! How can I help you today?',
    reasoningContent: '',
    isReasoningExpanded: false,
  },
];

// ========== UI 相关常量 ==========
export const DEBUG_TABS = {
  PREVIEW: 'preview',
  REQUEST: 'request',
  RESPONSE: 'response',
};

// ========== API 相关常量 ==========
export const API_ENDPOINTS = {
  CHAT_COMPLETIONS: '/pg/chat/completions',
  IMAGE_GENERATIONS: '/pg/images/generations',
  VIDEO_GENERATIONS: '/pg/video/generations',
  USER_MODELS: '/api/user/models',
  USER_GROUPS: '/api/user/self/groups',
};

export const MODEL_CAPABILITIES = {
  CHAT: 'chat',
  IMAGE: 'image',
  VIDEO: 'video',
};

const createDurationRange = (start, end) =>
  Array.from({ length: end - start + 1 }, (_, index) => start + index);

export const VIDEO_MODEL_CONFIG = {
  'video-2.0': {
    durations: createDurationRange(4, 15),
  },
  'video-2.0-fast': {
    durations: createDurationRange(4, 15),
  },
  sora2: {
    durations: [4, 8, 12],
  },
  veo31: {
    durations: [4, 6, 8],
  },
  'veo31-fast': {
    durations: [4, 6, 8],
  },
  'veo31-ref': {
    durations: [4, 6, 8],
  },
  'kling-v3': {
    durations: createDurationRange(3, 15),
  },
  'grok-imagine-video': {
    durations: [6, 10],
  },
  ko3: {
    durations: [4, 8, 12],
  },
};

const VIDEO_MODELS = new Set(Object.keys(VIDEO_MODEL_CONFIG));
const IMAGE_MODEL_KEYWORDS = ['flux', 'midjourney', 'nano-banana', 'gpt-image2'];

export const getModelCapability = (modelName = '') => {
  const normalizedModelName = modelName.toLowerCase();

  if (VIDEO_MODELS.has(normalizedModelName)) {
    return MODEL_CAPABILITIES.VIDEO;
  }
  if (
    IMAGE_MODEL_KEYWORDS.some((keyword) =>
      normalizedModelName.includes(keyword),
    )
  ) {
    return MODEL_CAPABILITIES.IMAGE;
  }
  return MODEL_CAPABILITIES.CHAT;
};

export const getPlaygroundEndpoint = (modelName = '') => {
  switch (getModelCapability(modelName)) {
    case MODEL_CAPABILITIES.VIDEO:
      return API_ENDPOINTS.VIDEO_GENERATIONS;
    case MODEL_CAPABILITIES.IMAGE:
      return API_ENDPOINTS.IMAGE_GENERATIONS;
    default:
      return API_ENDPOINTS.CHAT_COMPLETIONS;
  }
};

export const getVideoDurationOptions = (modelName = '') =>
  VIDEO_MODEL_CONFIG[modelName.toLowerCase()]?.durations || [4];

export const getValidVideoDuration = (modelName = '', duration) => {
  const durationOptions = getVideoDurationOptions(modelName);
  const numericDuration = Number(duration);

  return durationOptions.includes(numericDuration)
    ? numericDuration
    : durationOptions[0];
};

// ========== 配置默认值 ==========
export const DEFAULT_CONFIG = {
  inputs: {
    model: 'gpt-4o',
    group: '',
    temperature: 0.7,
    top_p: 1,
    max_tokens: 4096,
    frequency_penalty: 0,
    presence_penalty: 0,
    seed: null,
    duration: 4,
    stream: true,
    imageEnabled: false,
    imageUrls: [''],
  },
  parameterEnabled: {
    temperature: true,
    top_p: true,
    max_tokens: false,
    frequency_penalty: true,
    presence_penalty: true,
    seed: false,
  },
  systemPrompt: '',
  showDebugPanel: false,
  customRequestMode: false,
  customRequestBody: '',
};

// ========== 正则表达式 ==========
export const THINK_TAG_REGEX = /<think>([\s\S]*?)<\/think>/g;

// ========== 错误消息 ==========
export const ERROR_MESSAGES = {
  NO_TEXT_CONTENT: '此消息没有可复制的文本内容',
  INVALID_MESSAGE_TYPE: '无法复制此类型的消息内容',
  COPY_FAILED: '复制失败，请手动选择文本复制',
  COPY_HTTPS_REQUIRED: '复制功能需要 HTTPS 环境，请手动复制',
  BROWSER_NOT_SUPPORTED: '浏览器不支持复制功能，请手动复制',
  JSON_PARSE_ERROR: '自定义请求体格式错误，请检查JSON格式',
  API_REQUEST_ERROR: '请求发生错误',
  NETWORK_ERROR: '网络连接失败或服务器无响应',
};

// ========== 存储键名 ==========
export const STORAGE_KEYS = {
  CONFIG: 'playground_config',
  MESSAGES: 'playground_messages',
};
