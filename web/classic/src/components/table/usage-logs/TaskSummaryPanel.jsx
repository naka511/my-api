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
import { Card, Avatar, Skeleton } from '@douyinfe/semi-ui';
import { IconHistogram } from '@douyinfe/semi-icons';
import { Activity, Image, Video } from 'lucide-react';

const CARD_PROPS = {
  shadows: '',
  bordered: true,
  headerLine: true,
};

const STATUS_ITEMS = [
  { key: 'running', label: '进行中' },
  { key: 'success', label: '成功' },
  { key: 'failed', label: '失败' },
];

const EMPTY_COUNTS = { running: 0, success: 0, failed: 0 };

const normalizeCounts = (counts = {}) => ({
  running: counts.running || 0,
  success: counts.success || 0,
  failed: counts.failed || 0,
});

const getTotal = (counts) =>
  Object.values(counts).reduce((sum, value) => sum + value, 0);

const buildSummaryItems = (summary = {}) => [
  {
    key: 'total',
    title: '总任务',
    icon: <IconHistogram />,
    avatarColor: 'blue',
    counts: normalizeCounts(summary.total || EMPTY_COUNTS),
  },
  {
    key: 'image',
    title: '图片任务',
    icon: <Image size={14} />,
    avatarColor: 'purple',
    counts: normalizeCounts(summary.image || EMPTY_COUNTS),
  },
  {
    key: 'video',
    title: '视频任务',
    icon: <Video size={14} />,
    avatarColor: 'green',
    counts: normalizeCounts(summary.video || EMPTY_COUNTS),
  },
];

const TaskSummaryPanel = ({ t, loading = false, summary }) => {
  const items = buildSummaryItems(summary);

  return (
    <div className='mb-3'>
      <Card
        {...CARD_PROPS}
        className='!rounded-2xl'
        title={
          <div className='flex items-center gap-2'>
            <Activity size={16} />
            {t('任务汇总')}
          </div>
        }
        bodyStyle={{ padding: 16 }}
      >
        <div className='grid grid-cols-1 gap-4 lg:grid-cols-3'>
          {items.map((item) => {
            const total = getTotal(item.counts);
            return (
              <div
                key={item.key}
                className='rounded-xl p-4'
                style={{
                  border: '1px solid var(--semi-color-border)',
                  backgroundColor: 'var(--semi-color-bg-0)',
                }}
              >
                <div className='mb-4 flex items-center gap-3'>
                  <Avatar className='shrink-0' size='small' color={item.avatarColor}>
                    {item.icon}
                  </Avatar>
                  <div className='min-w-0'>
                    <div className='text-xs text-gray-500'>{t(item.title)}</div>
                    <div className='text-lg font-semibold leading-6 text-gray-800'>
                      <Skeleton
                        loading={loading}
                        active
                        placeholder={
                          <Skeleton.Paragraph
                            active
                            rows={1}
                            style={{
                              width: '48px',
                              height: '24px',
                              marginTop: '4px',
                            }}
                          />
                        }
                      >
                        {total}
                      </Skeleton>
                    </div>
                  </div>
                </div>

                <div className='grid grid-cols-3 gap-3'>
                  {STATUS_ITEMS.map((status) => (
                    <div
                      key={status.key}
                      className='rounded-xl px-3 py-3'
                      style={{ backgroundColor: 'var(--semi-color-fill-0)' }}
                    >
                      <div className='text-xs leading-5 text-slate-500'>
                        {t(status.label)}
                      </div>
                      <div className='mt-2 text-lg font-semibold leading-6 text-slate-900'>
                        {item.counts[status.key]}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </Card>
    </div>
  );
};

export default TaskSummaryPanel;
