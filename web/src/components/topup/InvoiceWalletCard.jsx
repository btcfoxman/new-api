import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Card,
  Empty,
  Input,
  Modal,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { FileText, ShieldCheck } from 'lucide-react';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';

const { Text } = Typography;

const SUBJECT_LABELS = {
  personal: '个人',
  company: '企业',
};

const SUBJECT_STATUS = {
  pending: { color: 'orange', label: '待审核' },
  verified: { color: 'green', label: '已认证' },
  rejected: { color: 'red', label: '已驳回' },
};

const INVOICE_STATUS = {
  pending: { color: 'orange', label: '待处理' },
  approved: { color: 'blue', label: '已受理' },
  issued: { color: 'green', label: '已开票' },
  rejected: { color: 'red', label: '已驳回' },
  cancelled: { color: 'grey', label: '已取消' },
  failed: { color: 'red', label: '开票失败' },
  red_pending: { color: 'pink', label: '红冲处理中' },
  red_issued: { color: 'violet', label: '已红冲' },
};

const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  alipay: '支付宝',
  wxpay: '微信',
  epay: '易支付',
  extpay: '扩展支付',
};

const formatMoney = (value) => `¥${Number(value || 0).toFixed(2)}`;

const renderTag = (map, status) => {
  const config = map[status] || { color: 'grey', label: status || '-' };
  return (
    <Tag color={config.color} shape='circle'>
      {config.label}
    </Tag>
  );
};

const dateToUnix = (value) => {
  if (!value) return 0;
  const date = new Date(`${value}T23:59:59`);
  if (Number.isNaN(date.getTime())) return 0;
  return Math.floor(date.getTime() / 1000);
};

const unixToDate = (value) => {
  if (!value) return '';
  const date = new Date(Number(value) * 1000);
  if (Number.isNaN(date.getTime())) return '';
  return date.toISOString().slice(0, 10);
};

const InvoiceWalletCard = ({ t, renderQuota }) => {
  const [activeKey, setActiveKey] = useState('topups');
  const [subject, setSubject] = useState(null);
  const [subjectLoading, setSubjectLoading] = useState(false);
  const [subjectModalVisible, setSubjectModalVisible] = useState(false);
  const [subjectSubmitting, setSubjectSubmitting] = useState(false);
  const [subjectForm, setSubjectForm] = useState({
    subject_type: 'company',
    real_name: '',
    company_name: '',
    id_no: '',
    tax_no: '',
    certificate_url: '',
    valid_until: '',
  });

  const [topups, setTopups] = useState([]);
  const [topupsLoading, setTopupsLoading] = useState(false);
  const [topupsTotal, setTopupsTotal] = useState(0);
  const [topupsPage, setTopupsPage] = useState(1);
  const [topupsPageSize, setTopupsPageSize] = useState(10);
  const [topupsKeyword, setTopupsKeyword] = useState('');
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [selectedRows, setSelectedRows] = useState([]);

  const [invoices, setInvoices] = useState([]);
  const [invoicesLoading, setInvoicesLoading] = useState(false);
  const [invoicesTotal, setInvoicesTotal] = useState(0);
  const [invoicesPage, setInvoicesPage] = useState(1);
  const [invoicesPageSize, setInvoicesPageSize] = useState(10);

  const [applyVisible, setApplyVisible] = useState(false);
  const [applyRows, setApplyRows] = useState([]);
  const [applyEmail, setApplyEmail] = useState('');
  const [applySubmitting, setApplySubmitting] = useState(false);

  const subjectVerified = subject?.status === 'verified';

  const invoiceAmount = useMemo(
    () => applyRows.reduce((sum, item) => sum + Number(item.money || 0), 0),
    [applyRows],
  );

  const loadSubject = async () => {
    setSubjectLoading(true);
    try {
      const res = await API.get('/api/user/invoices/subject');
      if (res.data?.success) {
        setSubject(res.data.data || null);
      } else {
        showError(res.data?.message || '加载认证信息失败');
      }
    } catch (e) {
      showError('加载认证信息失败');
    } finally {
      setSubjectLoading(false);
    }
  };

  const loadTopups = async (page = topupsPage, pageSize = topupsPageSize) => {
    setTopupsLoading(true);
    try {
      const qs =
        `p=${page}&page_size=${pageSize}` +
        (topupsKeyword ? `&keyword=${encodeURIComponent(topupsKeyword)}` : '');
      const res = await API.get(`/api/user/invoices/topups?${qs}`);
      if (res.data?.success) {
        setTopups(res.data.data?.items || []);
        setTopupsTotal(res.data.data?.total || 0);
      } else {
        showError(res.data?.message || '加载充值记录失败');
      }
    } catch (e) {
      showError('加载充值记录失败');
    } finally {
      setTopupsLoading(false);
    }
  };

  const loadInvoices = async (page = invoicesPage, pageSize = invoicesPageSize) => {
    setInvoicesLoading(true);
    try {
      const res = await API.get(`/api/user/invoices?p=${page}&page_size=${pageSize}`);
      if (res.data?.success) {
        setInvoices(res.data.data?.items || []);
        setInvoicesTotal(res.data.data?.total || 0);
      } else {
        showError(res.data?.message || '加载开票记录失败');
      }
    } catch (e) {
      showError('加载开票记录失败');
    } finally {
      setInvoicesLoading(false);
    }
  };

  useEffect(() => {
    loadSubject();
  }, []);

  useEffect(() => {
    if (activeKey === 'topups') {
      loadTopups(topupsPage, topupsPageSize);
    } else {
      loadInvoices(invoicesPage, invoicesPageSize);
    }
  }, [activeKey, topupsPage, topupsPageSize, invoicesPage, invoicesPageSize]);

  useEffect(() => {
    if (activeKey === 'topups') {
      loadTopups(1, topupsPageSize);
      setTopupsPage(1);
    }
  }, [topupsKeyword]);

  const openSubjectModal = () => {
    if (subjectVerified) return;
    setSubjectForm({
      subject_type: subject?.subject_type || 'company',
      real_name: subject?.real_name || '',
      company_name: subject?.company_name || '',
      id_no: '',
      tax_no: '',
      certificate_url: subject?.certificate_url || '',
      valid_until: unixToDate(subject?.valid_until),
    });
    setSubjectModalVisible(true);
  };

  const submitSubject = async () => {
    setSubjectSubmitting(true);
    try {
      const payload = {
        ...subjectForm,
        valid_until: dateToUnix(subjectForm.valid_until),
      };
      const res = await API.post('/api/user/invoices/subject', payload);
      if (res.data?.success) {
        showSuccess('认证信息已提交，请等待审核');
        setSubjectModalVisible(false);
        await loadSubject();
      } else {
        showError(res.data?.message || '提交认证失败');
      }
    } catch (e) {
      showError('提交认证失败');
    } finally {
      setSubjectSubmitting(false);
    }
  };

  const openApplyModal = (rows) => {
    if (!subjectVerified) {
      showError('请先完成实名认证');
      return;
    }
    if (!rows.length) {
      showError('请选择需要开票的充值记录');
      return;
    }
    setApplyRows(rows);
    setApplyVisible(true);
  };

  const submitApply = async () => {
    setApplySubmitting(true);
    try {
      const res = await API.post('/api/user/invoices/apply', {
        topup_ids: applyRows.map((item) => item.id),
        email: applyEmail,
      });
      if (res.data?.success) {
        showSuccess('开票申请已提交');
        setApplyVisible(false);
        setSelectedRowKeys([]);
        setSelectedRows([]);
        await loadTopups(topupsPage, topupsPageSize);
        await loadInvoices(1, invoicesPageSize);
      } else {
        showError(res.data?.message || '提交开票申请失败');
      }
    } catch (e) {
      showError('提交开票申请失败');
    } finally {
      setApplySubmitting(false);
    }
  };

  const cancelInvoice = (id) => {
    Modal.confirm({
      title: '取消开票申请',
      content: '仅待处理申请可取消，是否继续？',
      onOk: async () => {
        const res = await API.post(`/api/user/invoices/${id}/cancel`);
        if (res.data?.success) {
          showSuccess('已取消开票申请');
          loadInvoices(invoicesPage, invoicesPageSize);
          loadTopups(topupsPage, topupsPageSize);
        } else {
          showError(res.data?.message || '取消失败');
        }
      },
    });
  };

  const topupColumns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '用户ID', dataIndex: 'user_id', width: 90 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    {
      title: '充值时间',
      dataIndex: 'create_time',
      width: 170,
      render: (value) => timestamp2string(value),
    },
    {
      title: '充值额度',
      dataIndex: 'amount',
      width: 120,
      render: (value) => (renderQuota ? renderQuota(value) : value),
    },
    {
      title: '支付金额',
      dataIndex: 'money',
      width: 110,
      render: (value) => <Text type='danger'>{formatMoney(value)}</Text>,
    },
    {
      title: '支付方式',
      dataIndex: 'payment_method',
      width: 110,
      render: (value, record) =>
        PAYMENT_METHOD_MAP[record.payment_channel] ||
        PAYMENT_METHOD_MAP[value] ||
        record.payment_channel ||
        value ||
        '-',
    },
    {
      title: '订单号',
      dataIndex: 'trade_no',
      render: (value) => <Text copyable={{ content: value }}>{value}</Text>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (_, record) =>
        record.invoice_status
          ? renderTag(INVOICE_STATUS, record.invoice_status)
          : renderTag({ success: { color: 'green', label: '可开票' } }, 'success'),
    },
    {
      title: '操作',
      width: 110,
      render: (_, record) => (
        <Button
          size='small'
          type='primary'
          theme='outline'
          disabled={!record.invoice_available || !subjectVerified}
          onClick={() => openApplyModal([record])}
        >
          申请开票
        </Button>
      ),
    },
  ];

  const invoiceColumns = [
    { title: '申请ID', dataIndex: 'id', width: 90 },
    {
      title: '申请时间',
      dataIndex: 'created_at',
      width: 170,
      render: (value) => timestamp2string(value),
    },
    {
      title: '发票类型',
      dataIndex: 'invoice_type',
      width: 140,
      render: () => '增值税普通发票',
    },
    {
      title: '发票金额',
      dataIndex: 'amount',
      width: 120,
      render: (value) => <Text type='danger'>{formatMoney(value)}</Text>,
    },
    { title: 'Provider', dataIndex: 'provider_code', width: 130 },
    {
      title: '状态',
      dataIndex: 'status',
      width: 120,
      render: (value) => renderTag(INVOICE_STATUS, value),
    },
    { title: '发票号', dataIndex: 'invoice_no', width: 160, render: (value) => value || '-' },
    {
      title: '操作',
      width: 160,
      render: (_, record) => (
        <Space>
          {record.invoice_file_url ? (
            <Button
              size='small'
              type='primary'
              theme='outline'
              onClick={() => window.open(record.invoice_file_url, '_blank', 'noopener,noreferrer')}
            >
              下载
            </Button>
          ) : null}
          {record.status === 'pending' ? (
            <Button size='small' theme='outline' type='danger' onClick={() => cancelInvoice(record.id)}>
              取消
            </Button>
          ) : null}
        </Space>
      ),
    },
  ];

  return (
    <Card
      bordered={false}
      className='rounded-2xl shadow-sm'
      title={
        <div className='flex items-center gap-2'>
          <FileText size={18} />
          <span>{t ? t('发票管理') : '发票管理'}</span>
        </div>
      }
      headerExtraContent={
        <Button
          size='small'
          theme='borderless'
          loading={subjectLoading || topupsLoading || invoicesLoading}
          onClick={() => {
            loadSubject();
            activeKey === 'topups'
              ? loadTopups(topupsPage, topupsPageSize)
              : loadInvoices(invoicesPage, invoicesPageSize);
          }}
        >
          刷新
        </Button>
      }
    >
      <div className='mb-4 rounded-xl border border-dashed border-[var(--semi-color-border)] p-4 bg-[var(--semi-color-fill-0)]'>
        <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
          <div className='flex items-start gap-3'>
            <ShieldCheck size={22} className='mt-1 text-[var(--semi-color-primary)]' />
            <div>
              <div className='flex items-center gap-2 flex-wrap'>
                <Text strong>开票认证主体</Text>
                {subject ? renderTag(SUBJECT_STATUS, subject.status) : <Tag color='grey'>未认证</Tag>}
                {subject?.subject_type ? <Tag>{SUBJECT_LABELS[subject.subject_type]}</Tag> : null}
              </div>
              <div className='mt-2 text-sm text-[var(--semi-color-text-1)] space-y-1'>
                {subject ? (
                  <>
                    <div>
                      抬头：
                      {subject.subject_type === 'company'
                        ? subject.company_name || '-'
                        : subject.real_name || '-'}
                    </div>
                    <div>
                      证件：
                      {subject.subject_type === 'company'
                        ? subject.masked_tax_no || '-'
                        : subject.masked_id_no || '-'}
                    </div>
                    <div>有效期至：{subject.valid_until ? timestamp2string(subject.valid_until) : '-'}</div>
                    {subject.reject_reason ? <div className='text-red-500'>驳回原因：{subject.reject_reason}</div> : null}
                  </>
                ) : (
                  <div>申请开票前需要先完成企业或个人实名认证。</div>
                )}
              </div>
            </div>
          </div>
          <Button type='primary' theme='outline' disabled={subjectVerified} onClick={openSubjectModal}>
            {subject ? '更新认证' : '实名认证'}
          </Button>
        </div>
      </div>

      {!subjectVerified ? (
        <Banner
          type='warning'
          className='mb-4'
          description='发票信息必须与实名认证主体一致。认证通过后，在证件有效期内不允许修改，也不能在企业/个人类型之间切换。'
        />
      ) : null}

      <Tabs activeKey={activeKey} onChange={setActiveKey}>
        <Tabs.TabPane tab='充值记录' itemKey='topups'>
          <div className='mb-3 flex flex-col md:flex-row gap-3 md:items-center md:justify-between'>
            <Input
              prefix={<IconSearch />}
              showClear
              placeholder='搜索订单号'
              value={topupsKeyword}
              onChange={setTopupsKeyword}
              className='md:max-w-sm'
            />
            <Button
              type='primary'
              disabled={!subjectVerified || selectedRows.length === 0}
              onClick={() => openApplyModal(selectedRows)}
            >
              批量申请开票
            </Button>
          </div>
          <Table
            rowKey='id'
            size='small'
            columns={topupColumns}
            dataSource={topups}
            loading={topupsLoading}
            rowSelection={{
              selectedRowKeys,
              onChange: (keys, rows) => {
                setSelectedRowKeys(keys);
                setSelectedRows(rows.filter((row) => row.invoice_available));
              },
              getCheckboxProps: (record) => ({
                disabled: !record.invoice_available,
              }),
            }}
            pagination={{
              currentPage: topupsPage,
              pageSize: topupsPageSize,
              total: topupsTotal,
              showSizeChanger: true,
              onPageChange: setTopupsPage,
              onPageSizeChange: (size) => {
                setTopupsPageSize(size);
                setTopupsPage(1);
              },
            }}
            empty={<Empty description='暂无可展示充值记录' />}
          />
        </Tabs.TabPane>
        <Tabs.TabPane tab='开票记录' itemKey='invoices'>
          <Table
            rowKey='id'
            size='small'
            columns={invoiceColumns}
            dataSource={invoices}
            loading={invoicesLoading}
            pagination={{
              currentPage: invoicesPage,
              pageSize: invoicesPageSize,
              total: invoicesTotal,
              showSizeChanger: true,
              onPageChange: setInvoicesPage,
              onPageSizeChange: (size) => {
                setInvoicesPageSize(size);
                setInvoicesPage(1);
              },
            }}
            empty={<Empty description='暂无开票记录' />}
          />
        </Tabs.TabPane>
      </Tabs>

      <Modal
        title='实名认证'
        visible={subjectModalVisible}
        onCancel={() => setSubjectModalVisible(false)}
        onOk={submitSubject}
        confirmLoading={subjectSubmitting}
        maskClosable={false}
      >
        <div className='space-y-4'>
          <div>
            <Text strong>认证类型</Text>
            <select
              className='mt-2 w-full h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3'
              value={subjectForm.subject_type}
              onChange={(e) => setSubjectForm({ ...subjectForm, subject_type: e.target.value })}
            >
              <option value='company'>企业实名认证</option>
              <option value='personal'>个人实名认证</option>
            </select>
          </div>
          {subjectForm.subject_type === 'company' ? (
            <>
              <Input
                prefix='企业名称'
                value={subjectForm.company_name}
                onChange={(value) => setSubjectForm({ ...subjectForm, company_name: value })}
              />
              <Input
                prefix='纳税识别号'
                value={subjectForm.tax_no}
                onChange={(value) => setSubjectForm({ ...subjectForm, tax_no: value })}
              />
            </>
          ) : (
            <>
              <Input
                prefix='个人姓名'
                value={subjectForm.real_name}
                onChange={(value) => setSubjectForm({ ...subjectForm, real_name: value })}
              />
              <Input
                prefix='身份证号'
                value={subjectForm.id_no}
                onChange={(value) => setSubjectForm({ ...subjectForm, id_no: value })}
              />
            </>
          )}
          <Input
            prefix='凭证链接'
            placeholder='营业执照/身份证认证材料链接，可选'
            value={subjectForm.certificate_url}
            onChange={(value) => setSubjectForm({ ...subjectForm, certificate_url: value })}
          />
          <div>
            <Text strong>证件有效期</Text>
            <input
              type='date'
              className='mt-2 w-full h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3'
              value={subjectForm.valid_until}
              onChange={(e) => setSubjectForm({ ...subjectForm, valid_until: e.target.value })}
            />
          </div>
        </div>
      </Modal>

      <Modal
        title='申请开票'
        visible={applyVisible}
        onCancel={() => setApplyVisible(false)}
        onOk={submitApply}
        confirmLoading={applySubmitting}
        maskClosable={false}
      >
        <div className='space-y-4'>
          <Banner
            type='info'
            description='发票类型为增值税普通发票；发票抬头、证件号/税号将使用已认证信息，不能手动填写或修改。'
          />
          <div className='grid grid-cols-2 gap-3 text-sm'>
            <div>发票金额：{formatMoney(invoiceAmount)}</div>
            <div>订单数量：{applyRows.length}</div>
            <div>发票类型：增值税普通发票</div>
            <div>主体类型：{SUBJECT_LABELS[subject?.subject_type] || '-'}</div>
            <div className='col-span-2'>
              发票抬头：
              {subject?.subject_type === 'company' ? subject?.company_name : subject?.real_name}
            </div>
            <div className='col-span-2'>
              认证证件：
              {subject?.subject_type === 'company' ? subject?.masked_tax_no : subject?.masked_id_no}
            </div>
          </div>
          <Input
            prefix='接收邮箱'
            placeholder='用于接收电子发票'
            value={applyEmail}
            onChange={setApplyEmail}
          />
        </div>
      </Modal>
    </Card>
  );
};

export default InvoiceWalletCard;
