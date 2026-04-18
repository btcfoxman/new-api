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
import {
  calculateModelPrice,
  getGroupBillingMode,
  getModelPriceItems,
} from '../../../../../helpers';

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

  const getGroupRule = (group) => {
    const ruleMap = modelData?.group_pricing_rule;
    if (!ruleMap || typeof ruleMap !== 'object') return null;
    return ruleMap[String(group || '').trim().toLowerCase()] || null;
  };

  const getMatrixConfig = (group) => {
    const dimensions = getGroupRule(group)?.dimensions;
    if (!dimensions || dimensions.mode !== 'matrix') return null;
    if (!Array.isArray(dimensions.headers) || !Array.isArray(dimensions.rows)) {
      return null;
    }
    if (!Array.isArray(dimensions.prices) || dimensions.rows.length === 0) {
      return null;
    }
    return dimensions;
  };

  const getDimensionConfig = (group) => {
    const dimensions = getGroupRule(group)?.dimensions;
    if (!dimensions || typeof dimensions !== 'object') return null;
    if (dimensions.mode === 'matrix') return null;
    const hasResolution =
      dimensions.mode === 'resolution' &&
      dimensions.resolution_prices &&
      typeof dimensions.resolution_prices === 'object' &&
      Object.keys(dimensions.resolution_prices).length > 0;
    const hasDuration =
      dimensions.mode === 'duration' &&
      dimensions.duration_prices &&
      typeof dimensions.duration_prices === 'object' &&
      Object.keys(dimensions.duration_prices).length > 0;
    if (!hasResolution && !hasDuration) return null;
    return dimensions;
  };

  const formatMatrixPrice = (rawPrice, priceUnit, ratioValue) => {
    const numeric = Number(rawPrice);
    if (!Number.isFinite(numeric)) return '-';
    const finalValue = numeric * (Number(ratioValue) || 1);
    const text = finalValue.toFixed(3);
    if (priceUnit === 'per_second') {
      return `${t('每秒')} ${text}`;
    }
    return text;
  };

  const formatDimensionPrice = (rawPrice, ratioValue, suffix = '') => {
    const numeric = Number(rawPrice);
    if (!Number.isFinite(numeric)) return '-';
    const finalValue = numeric * (Number(ratioValue) || 1);
    return `${finalValue.toFixed(3)}${suffix}`;
  };

  const renderSmallText = (value, highlight = false) => (
    <span
      style={{ fontSize: 10 }}
      className={highlight ? 'font-semibold text-orange-600' : ''}
    >
      {value || '-'}
    </span>
  );

  const renderMatrixTable = (group, ratioValue) => {
    const matrix = getMatrixConfig(group);
    if (!matrix) return null;

    const headers = matrix.headers.map((header, index) => ({
      title: renderSmallText(header),
      dataIndex: `col_${index}`,
      key: `col_${index}`,
      render: (value) => renderSmallText(value || '-'),
    }));

    headers.push({
      title: renderSmallText(t('价格')),
      dataIndex: 'price_text',
      key: 'price_text',
      render: (value) => renderSmallText(value, true),
    });

    const rows = matrix.rows.map((row, index) => {
      const record = { key: `${group}-${index}` };
      row.forEach((value, colIndex) => {
        record[`col_${colIndex}`] = value;
      });
      record.price_text = formatMatrixPrice(
        matrix.prices[index],
        Array.isArray(matrix.price_units) ? matrix.price_units[index] : 'per_call',
        ratioValue,
      );
      return record;
    });

    return (
      <div className='mt-3' style={{ fontSize: 10 }}>
        <Table
          dataSource={rows}
          columns={headers}
          pagination={false}
          size='small'
          bordered={false}
          className='!rounded-lg'
        />
      </div>
    );
  };

  const renderDimensionTable = (group, ratioValue) => {
    const dimensions = getDimensionConfig(group);
    if (!dimensions) return null;

    if (dimensions.mode === 'resolution') {
      const rows = Object.entries(dimensions.resolution_prices || {}).map(
        ([resolution, price]) => ({
          key: `${group}-resolution-${resolution}`,
          dimension: resolution,
          priceText: formatDimensionPrice(price, ratioValue),
        }),
      );
      return (
        <div className='mt-3' style={{ fontSize: 10 }}>
          <Table
            dataSource={rows}
            columns={[
              {
                title: renderSmallText(t('分辨率')),
                dataIndex: 'dimension',
                key: 'dimension',
                render: (value) => renderSmallText(value),
              },
              {
                title: renderSmallText(t('价格')),
                dataIndex: 'priceText',
                key: 'priceText',
                render: (value) => renderSmallText(value, true),
              },
            ]}
            pagination={false}
            size='small'
            bordered={false}
            className='!rounded-lg'
          />
        </div>
      );
    }

    if (dimensions.mode === 'duration') {
      const multiplierMap =
        dimensions.resolution_multiplier &&
        typeof dimensions.resolution_multiplier === 'object'
          ? dimensions.resolution_multiplier
          : {};
      const rows = Object.entries(dimensions.duration_prices || {}).map(
        ([duration, price]) => ({
          key: `${group}-duration-${duration}`,
          dimension: `${duration}s`,
          priceText: formatDimensionPrice(price, ratioValue),
          multiplierText:
            Object.keys(multiplierMap).length > 0
              ? Object.entries(multiplierMap)
                  .map(([resolution, multiplier]) => `${resolution} x${multiplier}`)
                  .join(' / ')
              : '-',
        }),
      );
      return (
        <div className='mt-3' style={{ fontSize: 10 }}>
          <Table
            dataSource={rows}
            columns={[
              {
                title: renderSmallText(t('时长')),
                dataIndex: 'dimension',
                key: 'dimension',
                render: (value) => renderSmallText(value),
              },
              {
                title: renderSmallText(t('价格')),
                dataIndex: 'priceText',
                key: 'priceText',
                render: (value) => renderSmallText(value, true),
              },
              {
                title: renderSmallText(t('分辨率倍率')),
                dataIndex: 'multiplierText',
                key: 'multiplierText',
                render: (value) => renderSmallText(value),
              },
            ]}
            pagination={false}
            size='small'
            bordered={false}
            className='!rounded-lg'
          />
        </div>
      );
    }

    return null;
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
      const billingMode = getGroupBillingMode(modelData, group);
      const priceItems = getModelPriceItems(
        priceData,
        t,
        siteDisplayType,
        billingMode,
      );

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
        matrixTable: renderMatrixTable(group, groupRatioValue),
        dimensionTable: renderDimensionTable(group, groupRatioValue),
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
      render: (items, record) => {
        const hasDimensionTable = Boolean(
          record?.matrixTable || record?.dimensionTable,
        );
        return (
          <div className='space-y-1'>
            {!hasDimensionTable &&
              items.map((item) => (
                <div key={item.key}>
                  <div className='font-semibold text-orange-600'>
                    {item.label} {item.value}
                  </div>
                  <div className='text-xs text-gray-500'>{item.suffix}</div>
                </div>
              ))}
            {record?.matrixTable}
            {record?.dimensionTable}
          </div>
        );
      },
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
          <span className='text-sm'>-&gt;</span>
          {autoChain.map((g, idx) => (
            <React.Fragment key={g}>
              <Tag color='white' size='small' shape='circle'>
                {g}
                {t('分组')}
              </Tag>
              {idx < autoChain.length - 1 && <span className='text-sm'>-&gt;</span>}
            </React.Fragment>
          ))}
        </div>
      )}
      {renderGroupPriceTable()}
    </Card>
  );
};

export default ModelPricingTable;

