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
import { Card, Avatar, Typography, Table, Tag } from '@douyinfe/semi-ui';
import { IconCoinMoneyStroked } from '@douyinfe/semi-icons';
import { calculateModelPrice, getModelPriceItems } from '../../../../../helpers';

const { Text } = Typography;

const ModelPricingTable = ({
  modelData,
  groupRatio,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  showRatio,
  usableGroup,
  autoGroups = [],
  t,
}) => {
  const modelEnableGroups = Array.isArray(modelData?.enable_groups)
    ? modelData.enable_groups
    : [];
  const autoChain = autoGroups.filter((g) => modelEnableGroups.includes(g));

  const inferGroupBillingMode = (record, groupName) => {
    if (!record) return 'unknown';
    if (record.quota_type === 0) return 'per_token';
    if (record.quota_type !== 1) return 'unknown';

    const desc = String(record.description || '');
    const marker = '计费来源：';
    const idx = desc.indexOf(marker);
    if (idx < 0) return 'per_call';

    const sourceText = desc.slice(idx + marker.length);
    const parts = sourceText
      .split('；')
      .map((x) => x.trim())
      .filter(Boolean);
    const hit = parts.find((p) => p.startsWith(`${groupName}=`));
    if (!hit) return 'per_call';
    if (hit.includes('按秒')) return 'per_second';
    if (hit.includes('固定按次') || hit.includes('按次')) return 'per_call';
    return 'per_call';
  };

  const billingModeLabel = (mode) => {
    if (mode === 'per_token') return t('按量计费');
    if (mode === 'per_second') return t('按秒计费');
    if (mode === 'per_call') return t('按次计费');
    return '-';
  };

  const billingModeColor = (mode) => {
    if (mode === 'per_token') return 'violet';
    if (mode === 'per_second') return 'cyan';
    if (mode === 'per_call') return 'teal';
    return 'white';
  };

  const renderGroupPriceTable = () => {
    // 仅展示模型可用的分组：模型 enable_groups 与用户可用分组的交集
    const availableGroups = Object.keys(usableGroup || {})
      .filter((g) => g !== '')
      .filter((g) => g !== 'auto')
      .filter((g) => modelEnableGroups.includes(g))
      .sort((a, b) => {
        const rank = { default: 0, stable: 1, official: 2 };
        const ra = rank[a] ?? 99;
        const rb = rank[b] ?? 99;
        if (ra !== rb) return ra - rb;
        return a.localeCompare(b);
      });

    // 准备表格数据
    const tableData = availableGroups.map((group) => {
      const priceData = modelData
        ? calculateModelPrice({
            record: modelData,
            selectedGroup: group,
            groupRatio,
            tokenUnit,
            displayPrice,
            currency,
            quotaDisplayType: siteDisplayType,
          })
        : { inputPrice: '-', outputPrice: '-', price: '-' };
      const billingMode = inferGroupBillingMode(modelData, group);
      let priceItems = getModelPriceItems(priceData, t, siteDisplayType);
      if (billingMode === 'per_second' && modelData?.quota_type === 1) {
        priceItems = priceItems.map((item) =>
          item.key === 'fixed' ? { ...item, suffix: ` / ${t('秒')}` } : item,
        );
      }

      // 获取分组倍率
      const groupRatioValue =
        groupRatio && groupRatio[group] ? groupRatio[group] : 1;

      return {
        key: group,
        group: group,
        ratio: groupRatioValue,
        billingMode,
        billingType: billingModeLabel(billingMode),
        priceItems,
      };
    });

    // 定义表格列
    const columns = [
      {
        title: t('分组'),
        dataIndex: 'group',
        render: (text) => (
          <Tag color='white' size='small' shape='circle'>
            {text}
            {t('分组')}
          </Tag>
        ),
      },
    ];

    // 如果显示倍率，添加倍率列
    if (showRatio) {
      columns.push({
        title: t('倍率'),
        dataIndex: 'ratio',
        render: (text) => (
          <Tag color='white' size='small' shape='circle'>
            {text}x
          </Tag>
        ),
      });
    }

    // 添加计费类型列
    columns.push({
      title: t('计费类型'),
      dataIndex: 'billingType',
      render: (text, record) => {
        const color = billingModeColor(record?.billingMode);
        return (
          <Tag color={color} size='small' shape='circle'>
            {text || '-'}
          </Tag>
        );
      },
    });

    columns.push({
      title: siteDisplayType === 'TOKENS' ? t('计费摘要') : t('价格摘要'),
      dataIndex: 'priceItems',
      render: (items) => (
        <div className='space-y-1'>
          {items.map((item) => (
            <div key={item.key}>
              <div className='font-semibold text-orange-600'>
                {item.label} {item.value}
              </div>
              <div className='text-xs text-gray-500'>{item.suffix}</div>
            </div>
          ))}
        </div>
      ),
    });

    return (
      <Table
        dataSource={tableData}
        columns={columns}
        pagination={false}
        size='small'
        bordered={false}
        className='!rounded-lg'
      />
    );
  };

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='orange' className='mr-2 shadow-md'>
          <IconCoinMoneyStroked size={16} />
        </Avatar>
        <div>
          <Text className='text-lg font-medium'>{t('分组价格')}</Text>
          <div className='text-xs text-gray-600'>
            {t('不同用户分组的价格信息')}
          </div>
        </div>
      </div>
      {autoChain.length > 0 && (
        <div className='flex flex-wrap items-center gap-1 mb-4'>
          <span className='text-sm text-gray-600'>{t('auto分组调用链路')}</span>
          <span className='text-sm'>→</span>
          {autoChain.map((g, idx) => (
            <React.Fragment key={g}>
              <Tag color='white' size='small' shape='circle'>
                {g}
                {t('分组')}
              </Tag>
              {idx < autoChain.length - 1 && <span className='text-sm'>→</span>}
            </React.Fragment>
          ))}
        </div>
      )}
      {renderGroupPriceTable()}
    </Card>
  );
};

export default ModelPricingTable;
