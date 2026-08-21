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
import { Button, Form } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';

import { DATE_RANGE_PRESETS } from '../../../constants/console.constants';

const TaskLogsFilters = ({
  formInitValues,
  setFormApi,
  refresh,
  resetFilters,
  setShowColumnSelector,
  loading,
  isAdminUser,
  isRootUser,
  t,
}) => {
  return (
    <Form
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={refresh}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div className='flex flex-col gap-2'>
        <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-2'>
          {/* 时间选择器 */}
          <div className='col-span-1 lg:col-span-2'>
            <Form.DatePicker
              field='dateRange'
              className='w-full'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size='small'
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
            />
          </div>

          {/* 任务 ID */}
          <Form.Input
            field='task_id'
            prefix={<IconSearch />}
            placeholder={t('任务 ID')}
            showClear
            pure
            size='small'
          />

          {/* 渠道 ID - 仅管理员可见 */}
          {isAdminUser && (
            <Form.Input
              field='channel_id'
              prefix={<IconSearch />}
              placeholder={t('渠道 ID')}
              showClear
              pure
              size='small'
            />
          )}

          {/* 用户名称 - 仅超级管理员可见 */}
          {isRootUser && (
            <Form.Input
              field='username'
              prefix={<IconSearch />}
              placeholder={t('用户名称')}
              showClear
              pure
              size='small'
            />
          )}

          {/* 任务状态 - 仅超级管理员可见 */}
          {isRootUser && (
            <Form.Select
              field='task_status'
              placeholder={t('任务状态')}
              showClear
              pure
              size='small'
            >
              <Form.Select.Option value='SUCCESS'>{t('成功')}</Form.Select.Option>
              <Form.Select.Option value='IN_PROGRESS'>{t('执行中')}</Form.Select.Option>
              <Form.Select.Option value='QUEUED'>{t('队列中')}</Form.Select.Option>
              <Form.Select.Option value='SUBMITTED'>{t('已提交')}</Form.Select.Option>
              <Form.Select.Option value='NOT_START'>{t('未启动')}</Form.Select.Option>
              <Form.Select.Option value='FAILURE'>{t('失败')}</Form.Select.Option>
            </Form.Select>
          )}
        </div>

        {/* 操作按钮区域 */}
        <div className='flex justify-between items-center'>
          <div></div>
          <div className='flex gap-2'>
            <Button
              type='tertiary'
              htmlType='submit'
              loading={loading}
              size='small'
            >
              {t('查询')}
            </Button>
            <Button
              type='tertiary'
              onClick={resetFilters}
              size='small'
            >
              {t('重置')}
            </Button>
            <Button
              type='tertiary'
              onClick={() => setShowColumnSelector(true)}
              size='small'
            >
              {t('列设置')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default TaskLogsFilters;
