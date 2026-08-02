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

import React from 'react';
import { Progress, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import {
  Music,
  FileText,
  HelpCircle,
  CheckCircle,
  Pause,
  Clock,
  Play,
  XCircle,
  Loader,
  List,
  Hash,
  Video,
  Sparkles,
} from 'lucide-react';
import {
  TASK_ACTION_FIRST_TAIL_GENERATE,
  TASK_ACTION_GENERATE,
  TASK_ACTION_REFERENCE_GENERATE,
  TASK_ACTION_TEXT_GENERATE,
  TASK_ACTION_REMIX_GENERATE,
} from '../../../constants/common.constant';
import { CHANNEL_OPTIONS } from '../../../constants/channel.constants';
import { stringToColor } from '../../../helpers/render';
import { Avatar, Space } from '@douyinfe/semi-ui';

const colors = [
  'amber',
  'blue',
  'cyan',
  'green',
  'grey',
  'indigo',
  'light-blue',
  'lime',
  'orange',
  'pink',
  'purple',
  'red',
  'teal',
  'violet',
  'yellow',
];

// Render functions
const renderTimestamp = (timestampInSeconds) => {
  const date = new Date(timestampInSeconds * 1000); // 从秒转换为毫秒

  const year = date.getFullYear(); // 获取年份
  const month = ('0' + (date.getMonth() + 1)).slice(-2); // 获取月份，从0开始需要+1，并保证两位数
  const day = ('0' + date.getDate()).slice(-2); // 获取日期，并保证两位数
  const hours = ('0' + date.getHours()).slice(-2); // 获取小时，并保证两位数
  const minutes = ('0' + date.getMinutes()).slice(-2); // 获取分钟，并保证两位数
  const seconds = ('0' + date.getSeconds()).slice(-2); // 获取秒钟，并保证两位数

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`; // 格式化输出
};

function renderDuration(submit_time, finishTime) {
  if (!submit_time || !finishTime) return 'N/A';
  const durationSec = finishTime - submit_time;
  const color = durationSec > 60 ? 'red' : 'green';

  // 返回带有样式的颜色标签
  return (
    <Tag color={color} shape='circle'>
      {durationSec} s
    </Tag>
  );
}

const renderType = (type, t) => {
  switch (type) {
    case 'MUSIC':
      return (
        <Tag color='grey' shape='circle' prefixIcon={<Music size={14} />}>
          {t('生成音乐')}
        </Tag>
      );
    case 'LYRICS':
      return (
        <Tag color='pink' shape='circle' prefixIcon={<FileText size={14} />}>
          {t('生成歌词')}
        </Tag>
      );
    case TASK_ACTION_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('图生视频')}
        </Tag>
      );
    case TASK_ACTION_TEXT_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('文生视频')}
        </Tag>
      );
    case TASK_ACTION_FIRST_TAIL_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('首尾生视频')}
        </Tag>
      );
    case TASK_ACTION_REFERENCE_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('参照生视频')}
        </Tag>
      );
    case TASK_ACTION_REMIX_GENERATE:
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Sparkles size={14} />}>
          {t('视频Remix')}
        </Tag>
      );
    default:
      return (
        <Tag color='white' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {t('未知')}
        </Tag>
      );
  }
};

const getTaskModelName = (record) => {
  const properties = record?.properties || {};
  return String(
    properties.origin_model_name ||
      properties.upstream_model_name ||
      record?.model ||
      record?.model_name ||
      '',
  ).toLowerCase();
};

const getTaskModelDisplayName = (record) => {
  const properties = record?.properties || {};
  return (
    properties.origin_model_name ||
    record?.model ||
    record?.model_name ||
    properties.upstream_model_name ||
    '-'
  );
};

const truncateTaskContentPreview = (text, length = 12) => {
  const value = String(text || '').trim();
  if (!value) {
    return '';
  }
  const chars = Array.from(value);
  return chars.length > length ? `${chars.slice(0, length).join('')}...` : value;
};

const isUsablePromptPreview = (value) => {
  const text = String(value || '').trim();
  if (!text) {
    return false;
  }
  const lower = text.toLowerCase();
  return (
    !lower.startsWith('[large content omitted') &&
    !lower.startsWith('[truncated') &&
    !/^data:[^;]+;base64,/i.test(text) &&
    text.length <= 600
  );
};

const isUsablePromptText = (value) => {
  const text = String(value || '').trim();
  if (!text) {
    return false;
  }
  const lower = text.toLowerCase();
  return (
    !lower.startsWith('[large content omitted') &&
    !lower.startsWith('[truncated') &&
    !/^data:[^;]+;base64,/i.test(text)
  );
};

const getValueByPath = (source, path) => {
  if (!source || !path) {
    return undefined;
  }
  return path.split('.').reduce((current, key) => {
    if (current && typeof current === 'object') {
      return current[key];
    }
    return undefined;
  }, source);
};

const isPromptLikeKey = (key) => {
  const normalized = String(key || '').toLowerCase();
  return (
    normalized === 'prompt' ||
    normalized === 'input' ||
    normalized === 'text' ||
    normalized === 'content' ||
    normalized.endsWith('_prompt') ||
    normalized.includes('prompt_')
  );
};

const collectTaskPromptPreview = (source, depth = 0, key = '') => {
  if (!source || depth > 4) {
    return '';
  }
  if (typeof source === 'string') {
    const trimmed = source.trim();
    if ((trimmed.startsWith('{') || trimmed.startsWith('[')) && !isPromptLikeKey(key)) {
      try {
        const parsed = JSON.parse(trimmed);
        const preview = collectTaskPromptPreview(parsed, depth + 1, key);
        if (preview) {
          return preview;
        }
      } catch (error) {
        // Ignore invalid JSON strings; they may be plain prompt-like text.
      }
    }
    return isPromptLikeKey(key) && isUsablePromptText(source) ? source : '';
  }
  if (Array.isArray(source)) {
    for (const item of source) {
      const preview = collectTaskPromptPreview(item, depth + 1, key);
      if (preview) {
        return preview;
      }
    }
    return '';
  }
  if (typeof source !== 'object') {
    return '';
  }

  const promptKeys = ['prompt', 'input', 'text', 'content'];
  for (const promptKey of promptKeys) {
    if (isUsablePromptText(source[promptKey])) {
      return source[promptKey];
    }
  }
  for (const [childKey, value] of Object.entries(source)) {
    const preview = collectTaskPromptPreview(value, depth + 1, childKey);
    if (preview) {
      return preview;
    }
  }
  return '';
};

const getTaskContentPreview = (record, t) => {
  const explicitPreview = record?.content_preview;
  if (isUsablePromptPreview(explicitPreview)) {
    return explicitPreview;
  }

  const candidates = [
    getValueByPath(record, 'properties.input'),
    getValueByPath(record, 'data.prompt'),
    getValueByPath(record, 'data.input'),
    getValueByPath(record, 'data.text'),
    getValueByPath(record, 'data.content'),
    getValueByPath(record, 'data.data.prompt'),
    getValueByPath(record, 'data.data.input'),
    getValueByPath(record, 'data.properties.input'),
    collectTaskPromptPreview(record?.data),
  ];
  const fallback = candidates.find(isUsablePromptPreview);
  return fallback ? truncateTaskContentPreview(fallback) : t('查看');
};

const isVideoPreviewTask = (record, resultUrl) => {
  const action = String(record?.action || '').toLowerCase();
  const modelName = getTaskModelName(record);
  const url = String(resultUrl || record?.result_url || '').toLowerCase();
  const knownVideoActions = [
    TASK_ACTION_GENERATE,
    TASK_ACTION_TEXT_GENERATE,
    TASK_ACTION_FIRST_TAIL_GENERATE,
    TASK_ACTION_REFERENCE_GENERATE,
    TASK_ACTION_REMIX_GENERATE,
  ].map((item) => item.toLowerCase());

  return (
    knownVideoActions.includes(action) ||
    action.includes('video') ||
    action.includes('generate') ||
    modelName.includes('video') ||
    modelName.includes('sora') ||
    modelName.includes('veo') ||
    modelName.includes('ko3') ||
    url.includes('/v1/videos/') ||
    /\.(mp4|webm|mov|m4v)(\?|#|$)/.test(url)
  );
};

const renderPlatform = (platform, t) => {
  let option = CHANNEL_OPTIONS.find(
    (opt) => String(opt.value) === String(platform),
  );
  if (option) {
    return (
      <Tag color={option.color} shape='circle'>
        {option.label}
      </Tag>
    );
  }
  switch (platform) {
    case 'suno':
      return (
        <Tag color='green' shape='circle'>
          Suno
        </Tag>
      );
    default:
      return (
        <Tag color='white' shape='circle'>
          {t('未知')}
        </Tag>
      );
  }
};

const renderStatus = (type, t) => {
  switch (type) {
    case 'SUCCESS':
      return (
        <Tag
          color='green'
          shape='circle'
          prefixIcon={<CheckCircle size={14} />}
        >
          {t('成功')}
        </Tag>
      );
    case 'NOT_START':
      return (
        <Tag color='grey' shape='circle' prefixIcon={<Pause size={14} />}>
          {t('未启动')}
        </Tag>
      );
    case 'SUBMITTED':
      return (
        <Tag color='yellow' shape='circle' prefixIcon={<Clock size={14} />}>
          {t('队列中')}
        </Tag>
      );
    case 'IN_PROGRESS':
      return (
        <Tag color='blue' shape='circle' prefixIcon={<Play size={14} />}>
          {t('执行中')}
        </Tag>
      );
    case 'FAILURE':
      return (
        <Tag color='red' shape='circle' prefixIcon={<XCircle size={14} />}>
          {t('失败')}
        </Tag>
      );
    case 'QUEUED':
      return (
        <Tag color='yellow' shape='circle' prefixIcon={<List size={14} />}>
          {t('队列中')}
        </Tag>
      );
    case 'UNKNOWN':
      return (
        <Tag color='white' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {t('未知')}
        </Tag>
      );
    case '':
      return (
        <Tag color='grey' shape='circle' prefixIcon={<Loader size={14} />}>
          {t('正在提交')}
        </Tag>
      );
    default:
      return (
        <Tag color='white' shape='circle' prefixIcon={<HelpCircle size={14} />}>
          {t('未知')}
        </Tag>
      );
  }
};

export const getTaskLogsColumns = ({
  t,
  COLUMN_KEYS,
  copyText,
  openContentModal,
  isAdminUser,
  isRootUser,
  openTaskContentModal,
  openVideoModal,
  openAudioModal,
}) => {
  return [
    {
      key: COLUMN_KEYS.SUBMIT_TIME,
      title: t('提交时间'),
      dataIndex: 'submit_time',
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      key: COLUMN_KEYS.FINISH_TIME,
      title: t('结束时间'),
      dataIndex: 'finish_time',
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      key: COLUMN_KEYS.DURATION,
      title: t('花费时间'),
      dataIndex: 'finish_time',
      render: (finish, record) => {
        return <>{finish ? renderDuration(record.submit_time, finish) : '-'}</>;
      },
    },
    {
      key: COLUMN_KEYS.CHANNEL,
      title: t('渠道'),
      dataIndex: 'channel_id',
      render: (text, record, index) => {
        return isAdminUser ? (
          <div>
            <Tag
              color={colors[parseInt(text) % colors.length]}
              size='large'
              shape='circle'
              onClick={() => {
                copyText(text);
              }}
            >
              {text}
            </Tag>
          </div>
        ) : (
          <></>
        );
      },
    },
    {
      key: COLUMN_KEYS.USERNAME,
      title: t('用户'),
      dataIndex: 'username',
      render: (userId, record, index) => {
        if (!isAdminUser) {
          return <></>;
        }
        const displayText = String(record.username || userId || '?');
        return (
          <Space>
            <Avatar
              size='extra-small'
              color={stringToColor(displayText)}
            >
              {displayText.slice(0, 1)}
            </Avatar>
            <Typography.Text>
              {displayText}
            </Typography.Text>
          </Space>
        );
      },
    },
    {
      key: COLUMN_KEYS.PLATFORM,
      title: t('平台'),
      dataIndex: 'platform',
      render: (text, record, index) => {
        return <div>{renderPlatform(text, t)}</div>;
      },
    },
    {
      key: COLUMN_KEYS.MODEL,
      title: t('模型'),
      dataIndex: 'properties',
      render: (text, record, index) => {
        const modelName = getTaskModelDisplayName(record);
        return (
          <Typography.Text ellipsis={{ showTooltip: true }}>
            <div>{modelName}</div>
          </Typography.Text>
        );
      },
    },
    {
      key: COLUMN_KEYS.TYPE,
      title: t('类型'),
      dataIndex: 'action',
      render: (text, record, index) => {
        return <div>{renderType(text, t)}</div>;
      },
    },
    {
      key: COLUMN_KEYS.TASK_ID,
      title: t('任务ID'),
      dataIndex: 'task_id',
      render: (text, record, index) => {
        return (
          <Typography.Text
            ellipsis={{ showTooltip: true }}
            onClick={() => {
              openContentModal(JSON.stringify(record, null, 2));
            }}
          >
            <div>{text}</div>
          </Typography.Text>
        );
      },
    },
    {
      key: COLUMN_KEYS.TASK_CONTENT,
      title: t('任务内容'),
      dataIndex: 'task_id',
      render: (text, record, index) => {
        if (!isRootUser) {
          return <></>;
        }
        const previewText = getTaskContentPreview(record, t);
        return (
          <a
            href='#'
            title={previewText}
            onClick={(e) => {
              e.preventDefault();
              openTaskContentModal(text);
            }}
          >
            {previewText}
          </a>
        );
      },
    },
    {
      key: COLUMN_KEYS.TASK_STATUS,
      title: t('任务状态'),
      dataIndex: 'status',
      render: (text, record, index) => {
        return <div>{renderStatus(text, t)}</div>;
      },
    },
    {
      key: COLUMN_KEYS.PROGRESS,
      title: t('进度'),
      dataIndex: 'progress',
      render: (text, record, index) => {
        return (
          <div>
            {isNaN(text?.replace('%', '')) ? (
              text || '-'
            ) : (
              <Progress
                stroke={
                  record.status === 'FAILURE'
                    ? 'var(--semi-color-warning)'
                    : null
                }
                percent={text ? parseInt(text.replace('%', '')) : 0}
                showInfo={true}
                aria-label='task progress'
                style={{ minWidth: '160px' }}
              />
            )}
          </div>
        );
      },
    },
    {
      key: COLUMN_KEYS.FAIL_REASON,
      title: t('详情'),
      dataIndex: 'fail_reason',
      fixed: 'right',
      render: (text, record, index) => {
        // Suno audio preview
        const isSunoSuccess =
          record.platform === 'suno' &&
          record.status === 'SUCCESS' &&
          Array.isArray(record.data) &&
          record.data.some((c) => c.audio_url);
        if (isSunoSuccess) {
          return (
            <a
              href='#'
              onClick={(e) => {
                e.preventDefault();
                openAudioModal(record.data);
              }}
            >
              {t('点击预览音乐')}
            </a>
          );
        }

        // 视频预览：优先走同站代理，避免 http/IP/跨域链接影响播放与下载。
        const isSuccess = record.status === 'SUCCESS';
        const taskId = record.task_id || record.TaskID || record.id;
        const proxyUrl = taskId
          ? `/v1/videos/${encodeURIComponent(taskId)}/content`
          : record.result_url;
        const resultUrl = proxyUrl || record.result_url;
        const isVideoTask = isVideoPreviewTask(record, resultUrl);
        const hasResultUrl = typeof resultUrl === 'string' && resultUrl.length > 0;
        if (isSuccess && isVideoTask && hasResultUrl) {
          return (
            <a
              href='#'
              onClick={(e) => {
                e.preventDefault();
                openVideoModal(resultUrl);
              }}
            >
              {t('点击预览视频')}
            </a>
          );
        }
        if (!text) {
          return t('无');
        }
        return (
          <Typography.Text
            ellipsis={{ showTooltip: true }}
            style={{ width: 100 }}
            onClick={() => {
              openContentModal(text);
            }}
          >
            {text}
          </Typography.Text>
        );
      },
    },
  ];
};
