import React, { useEffect, useMemo, useState } from 'react';
import { Banner, Button, Card, Checkbox, Empty, Input, Modal, Space, Switch, Table, Tabs, Tag, Typography } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import { FileText, ShieldCheck } from 'lucide-react';
import { API, isAdmin, isRoot, showError, showSuccess, timestamp2string } from '../../helpers';

const { Text } = Typography;

const SUBJECT_LABELS = { personal: '个人', company: '企业' };
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
const PAYMENT_METHOD_MAP = { stripe: 'Stripe', creem: 'Creem', waffo: 'Waffo', alipay: '支付宝', wxpay: '微信', wechat: '微信', epay: '易支付', extpay: '扩展支付' };
const PROVIDER_LABELS = { manual: '人工开票', alipay_invoice: '支付宝开票', wechat_invoice: '微信开票', fapiao_cloud: '发票云', nuonuo: '诺诺开放平台' };

const formatMoney = (value) => `¥${Number(value || 0).toFixed(2)}`;
const formatValidUntil = (value) => (value ? timestamp2string(value) : '长期');
const renderTag = (map, status) => {
  const config = map[status] || { color: 'grey', label: status || '-' };
  return <Tag color={config.color} shape='circle'>{config.label}</Tag>;
};
const dateToUnix = (value) => {
  if (!value) return 0;
  const date = new Date(`${value}T23:59:59`);
  return Number.isNaN(date.getTime()) ? 0 : Math.floor(date.getTime() / 1000);
};
const unixToDate = (value) => {
  if (!value) return '';
  const date = new Date(Number(value) * 1000);
  return Number.isNaN(date.getTime()) ? '' : date.toISOString().slice(0, 10);
};
const pageQuery = (page, pageSize, extras = {}) => {
  const params = new URLSearchParams({ p: String(page), page_size: String(pageSize) });
  Object.entries(extras).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') params.set(key, value);
  });
  return params.toString();
};
const textOfSubject = (record) => (record?.subject_type === 'company' ? record?.company_name : record?.real_name) || '-';
const docOfSubject = (record) => (record?.subject_type === 'company' ? (record?.tax_no || record?.masked_tax_no) : (record?.id_no || record?.masked_id_no)) || '-';
const taxNoOfSubject = (record) => (record?.subject_type === 'company' ? (record?.tax_no || record?.masked_tax_no) : '') || '-';

const InvoiceWalletCard = ({ t, renderQuota, invoiceEnabled = false, invoiceVisibleUserIds = '' }) => {
  const adminUser = isAdmin();
  const rootUser = isRoot();
  const [featureEnabled, setFeatureEnabled] = useState(!!invoiceEnabled);
  const [visibleUserIds, setVisibleUserIds] = useState(invoiceVisibleUserIds || '');
  const featureVisible = featureEnabled || adminUser || rootUser;
  const [visibleUserIdsSaving, setVisibleUserIdsSaving] = useState(false);

  const [activeKey, setActiveKey] = useState('topups');
  const [subject, setSubject] = useState(null);
  const [subjectLoading, setSubjectLoading] = useState(false);
  const [subjectModalVisible, setSubjectModalVisible] = useState(false);
  const [subjectSubmitting, setSubjectSubmitting] = useState(false);
  const [subjectForm, setSubjectForm] = useState({ subject_type: 'company', real_name: '', company_name: '', id_no: '', tax_no: '', certificate_url: '', valid_until: '', valid_forever: true });

  const [topups, setTopups] = useState([]);
  const [topupsLoading, setTopupsLoading] = useState(false);
  const [topupsTotal, setTopupsTotal] = useState(0);
  const [topupsPage, setTopupsPage] = useState(1);
  const [topupsPageSize, setTopupsPageSize] = useState(10);
  const [topupsKeyword, setTopupsKeyword] = useState('');
  const [invoiceTargetUserId, setInvoiceTargetUserId] = useState('');
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

  const [adminSubjects, setAdminSubjects] = useState([]);
  const [adminSubjectsLoading, setAdminSubjectsLoading] = useState(false);
  const [adminSubjectsTotal, setAdminSubjectsTotal] = useState(0);
  const [adminSubjectsPage, setAdminSubjectsPage] = useState(1);
  const [adminSubjectsPageSize, setAdminSubjectsPageSize] = useState(10);
  const [adminSubjectStatus, setAdminSubjectStatus] = useState('pending');
  const [adminSubjectKeyword, setAdminSubjectKeyword] = useState('');
  const [reviewModal, setReviewModal] = useState({ visible: false, record: null, status: 'verified', reason: '' });
  const [reviewSubmitting, setReviewSubmitting] = useState(false);

  const [adminInvoices, setAdminInvoices] = useState([]);
  const [adminInvoicesLoading, setAdminInvoicesLoading] = useState(false);
  const [adminInvoicesTotal, setAdminInvoicesTotal] = useState(0);
  const [adminInvoicesPage, setAdminInvoicesPage] = useState(1);
  const [adminInvoicesPageSize, setAdminInvoicesPageSize] = useState(10);
  const [adminInvoiceStatus, setAdminInvoiceStatus] = useState('pending');
  const [adminInvoiceKeyword, setAdminInvoiceKeyword] = useState('');
  const [invoiceUpdateModal, setInvoiceUpdateModal] = useState({ visible: false, record: null, form: {} });
  const [invoiceUpdateSubmitting, setInvoiceUpdateSubmitting] = useState(false);

  const [providers, setProviders] = useState([]);
  const [providersLoading, setProvidersLoading] = useState(false);
  const [providerModal, setProviderModal] = useState({ visible: false, form: {} });
  const [providerSubmitting, setProviderSubmitting] = useState(false);
  const [featureSwitchLoading, setFeatureSwitchLoading] = useState(false);

  const subjectVerified = subject?.status === 'verified';
  const invoiceAmount = useMemo(() => applyRows.reduce((sum, item) => sum + Number(item.money || 0), 0), [applyRows]);
  const targetUserQuery = () => {
    const userId = Number(String(invoiceTargetUserId || '').trim());
    return adminUser && Number.isInteger(userId) && userId > 0 ? { user_id: userId } : {};
  };

  useEffect(() => setFeatureEnabled(!!invoiceEnabled), [invoiceEnabled]);
  useEffect(() => setVisibleUserIds(invoiceVisibleUserIds || ''), [invoiceVisibleUserIds]);

  const loadSubject = async () => {
    setSubjectLoading(true);
    try {
      const query = new URLSearchParams(targetUserQuery()).toString();
      const res = await API.get(`/api/user/invoices/subject${query ? `?${query}` : ''}`);
      if (res.data?.success) setSubject(res.data.data || null);
      else showError(res.data?.message || '加载认证信息失败');
    } catch (e) {
      showError('加载认证信息失败');
    } finally {
      setSubjectLoading(false);
    }
  };
  const loadTopups = async (page = topupsPage, pageSize = topupsPageSize) => {
    setTopupsLoading(true);
    try {
      const res = await API.get(`/api/user/invoices/topups?${pageQuery(page, pageSize, { keyword: topupsKeyword, ...targetUserQuery() })}`);
      if (res.data?.success) {
        setTopups(res.data.data?.items || []);
        setTopupsTotal(res.data.data?.total || 0);
      } else showError(res.data?.message || '加载充值记录失败');
    } catch (e) {
      showError('加载充值记录失败');
    } finally {
      setTopupsLoading(false);
    }
  };
  const loadInvoices = async (page = invoicesPage, pageSize = invoicesPageSize) => {
    setInvoicesLoading(true);
    try {
      const res = await API.get(`/api/user/invoices?${pageQuery(page, pageSize, targetUserQuery())}`);
      if (res.data?.success) {
        setInvoices(res.data.data?.items || []);
        setInvoicesTotal(res.data.data?.total || 0);
      } else showError(res.data?.message || '加载开票记录失败');
    } catch (e) {
      showError('加载开票记录失败');
    } finally {
      setInvoicesLoading(false);
    }
  };
  const loadAdminSubjects = async (page = adminSubjectsPage, pageSize = adminSubjectsPageSize) => {
    if (!adminUser) return;
    setAdminSubjectsLoading(true);
    try {
      const res = await API.get(`/api/user/admin/invoice-subjects?${pageQuery(page, pageSize, { status: adminSubjectStatus, keyword: adminSubjectKeyword })}`);
      if (res.data?.success) {
        setAdminSubjects(res.data.data?.items || []);
        setAdminSubjectsTotal(res.data.data?.total || 0);
      } else showError(res.data?.message || '加载认证审核列表失败');
    } catch (e) {
      showError('加载认证审核列表失败');
    } finally {
      setAdminSubjectsLoading(false);
    }
  };
  const loadAdminInvoices = async (page = adminInvoicesPage, pageSize = adminInvoicesPageSize) => {
    if (!adminUser) return;
    setAdminInvoicesLoading(true);
    try {
      const res = await API.get(`/api/user/admin/invoices?${pageQuery(page, pageSize, { status: adminInvoiceStatus, keyword: adminInvoiceKeyword })}`);
      if (res.data?.success) {
        setAdminInvoices(res.data.data?.items || []);
        setAdminInvoicesTotal(res.data.data?.total || 0);
      } else showError(res.data?.message || '加载开票管理列表失败');
    } catch (e) {
      showError('加载开票管理列表失败');
    } finally {
      setAdminInvoicesLoading(false);
    }
  };
  const loadProviders = async () => {
    if (!adminUser) return;
    setProvidersLoading(true);
    try {
      const res = await API.get('/api/user/admin/invoice-provider-configs');
      if (res.data?.success) setProviders(res.data.data || []);
      else showError(res.data?.message || '加载开票渠道配置失败');
    } catch (e) {
      showError('加载开票渠道配置失败');
    } finally {
      setProvidersLoading(false);
    }
  };
  const refreshActive = () => {
    loadSubject();
    if (activeKey === 'topups') loadTopups(topupsPage, topupsPageSize);
    if (activeKey === 'invoices') loadInvoices(invoicesPage, invoicesPageSize);
    if (activeKey === 'subject_admin') loadAdminSubjects(adminSubjectsPage, adminSubjectsPageSize);
    if (activeKey === 'invoice_admin') loadAdminInvoices(adminInvoicesPage, adminInvoicesPageSize);
    if (activeKey === 'provider_admin') loadProviders();
  };

  useEffect(() => {
    if (!featureVisible) return;
    loadSubject();
  }, [featureVisible, invoiceTargetUserId]);
  useEffect(() => {
    if (!featureVisible) return;
    if (activeKey === 'topups') loadTopups(topupsPage, topupsPageSize);
    if (activeKey === 'invoices') loadInvoices(invoicesPage, invoicesPageSize);
    if (activeKey === 'subject_admin') loadAdminSubjects(adminSubjectsPage, adminSubjectsPageSize);
    if (activeKey === 'invoice_admin') loadAdminInvoices(adminInvoicesPage, adminInvoicesPageSize);
    if (activeKey === 'provider_admin') loadProviders();
  }, [featureVisible, activeKey, topupsPage, topupsPageSize, invoicesPage, invoicesPageSize, adminSubjectsPage, adminSubjectsPageSize, adminInvoicesPage, adminInvoicesPageSize]);
  useEffect(() => {
    if (!featureVisible || activeKey !== 'topups') return;
    setSelectedRowKeys([]);
    setSelectedRows([]);
    setTopupsPage(1);
    loadTopups(1, topupsPageSize);
  }, [topupsKeyword, invoiceTargetUserId]);
  useEffect(() => {
    if (!featureVisible || activeKey !== 'invoices') return;
    setInvoicesPage(1);
    loadInvoices(1, invoicesPageSize);
  }, [invoiceTargetUserId]);
  useEffect(() => {
    if (!featureVisible || activeKey !== 'subject_admin') return;
    setAdminSubjectsPage(1);
    loadAdminSubjects(1, adminSubjectsPageSize);
  }, [adminSubjectStatus, adminSubjectKeyword]);
  useEffect(() => {
    if (!featureVisible || activeKey !== 'invoice_admin') return;
    setAdminInvoicesPage(1);
    loadAdminInvoices(1, adminInvoicesPageSize);
  }, [adminInvoiceStatus, adminInvoiceKeyword]);

  const toggleFeatureEnabled = async (checked) => {
    setFeatureSwitchLoading(true);
    try {
      const res = await API.put('/api/option/', { key: 'InvoiceEnabled', value: checked ? 'true' : 'false' });
      if (res.data?.success) {
        setFeatureEnabled(checked);
        showSuccess(checked ? '发票功能已开启' : '发票功能已关闭');
      } else showError(res.data?.message || '更新发票功能开关失败');
    } catch (e) {
      showError('更新发票功能开关失败');
    } finally {
      setFeatureSwitchLoading(false);
    }
  };
  const saveVisibleUserIds = async () => {
    setVisibleUserIdsSaving(true);
    try {
      const res = await API.put('/api/option/', { key: 'InvoiceVisibleUserIds', value: visibleUserIds || '' });
      if (res.data?.success) showSuccess('发票可见用户已保存');
      else showError(res.data?.message || '保存发票可见用户失败');
    } catch (e) {
      showError('保存发票可见用户失败');
    } finally {
      setVisibleUserIdsSaving(false);
    }
  };
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
      valid_forever: !subject?.valid_until,
    });
    setSubjectModalVisible(true);
  };
  const submitSubject = async () => {
    if (!subjectForm.valid_forever && !subjectForm.valid_until) {
      showError('请选择证件有效期，或勾选长期有效');
      return;
    }
    setSubjectSubmitting(true);
    try {
      const payload = { ...subjectForm, valid_until: subjectForm.valid_forever ? 0 : dateToUnix(subjectForm.valid_until) };
      delete payload.valid_forever;
      const res = await API.post('/api/user/invoices/subject', payload);
      if (res.data?.success) {
        showSuccess('认证信息已提交，请等待审核');
        setSubjectModalVisible(false);
        await loadSubject();
      } else showError(res.data?.message || '提交认证失败');
    } catch (e) {
      showError('提交认证失败');
    } finally {
      setSubjectSubmitting(false);
    }
  };
  const openApplyModal = (rows) => {
    if (!subjectVerified) return showError('请先完成实名认证');
    if (!rows.length) return showError('请选择需要开票的充值记录');
    setApplyRows(rows);
    setApplyEmail(subject?.default_email || applyEmail || '');
    setApplyVisible(true);
  };
  const submitApply = async () => {
    setApplySubmitting(true);
    try {
      const res = await API.post('/api/user/invoices/apply', { topup_ids: applyRows.map((item) => item.id), email: applyEmail, ...targetUserQuery() });
      if (res.data?.success) {
        showSuccess('开票申请已提交');
        setSubject((prev) => (prev ? { ...prev, default_email: applyEmail } : prev));
        setApplyVisible(false);
        setSelectedRowKeys([]);
        setSelectedRows([]);
        await loadTopups(topupsPage, topupsPageSize);
        await loadInvoices(1, invoicesPageSize);
      } else showError(res.data?.message || '提交开票申请失败');
    } catch (e) {
      showError('提交开票申请失败');
    } finally {
      setApplySubmitting(false);
    }
  };
  const cancelInvoice = (id) => Modal.confirm({
    title: '取消开票申请',
    content: '仅待处理申请可取消，是否继续？',
    onOk: async () => {
      const res = await API.post(`/api/user/invoices/${id}/cancel`);
      if (res.data?.success) {
        showSuccess('已取消开票申请');
        loadInvoices(invoicesPage, invoicesPageSize);
        loadTopups(topupsPage, topupsPageSize);
      } else showError(res.data?.message || '取消失败');
    },
  });
  const openReviewModal = (record, status) => setReviewModal({ visible: true, record, status, reason: status === 'rejected' ? record.reject_reason || '' : '' });
  const submitReview = async () => {
    if (reviewModal.status === 'rejected' && !reviewModal.reason.trim()) return showError('驳回时必须填写原因');
    setReviewSubmitting(true);
    try {
      const res = await API.put(`/api/user/admin/invoice-subjects/${reviewModal.record.id}/review`, { status: reviewModal.status, reject_reason: reviewModal.reason });
      if (res.data?.success) {
        showSuccess('认证审核已更新');
        setReviewModal({ visible: false, record: null, status: 'verified', reason: '' });
        loadAdminSubjects(adminSubjectsPage, adminSubjectsPageSize);
      } else showError(res.data?.message || '认证审核失败');
    } catch (e) {
      showError('认证审核失败');
    } finally {
      setReviewSubmitting(false);
    }
  };
  const openInvoiceUpdateModal = (record) => setInvoiceUpdateModal({ visible: true, record, form: { status: record.status || 'approved', invoice_no: record.invoice_no || '', invoice_file_url: record.invoice_file_url || '', file_type: record.file_type || 'pdf', reject_reason: record.reject_reason || '', send_email: true } });
  const submitInvoiceUpdate = async () => {
    setInvoiceUpdateSubmitting(true);
    try {
      const res = await API.put(`/api/user/admin/invoices/${invoiceUpdateModal.record.id}`, invoiceUpdateModal.form);
      if (res.data?.success) {
        showSuccess('开票状态已更新');
        setInvoiceUpdateModal({ visible: false, record: null, form: {} });
        loadAdminInvoices(adminInvoicesPage, adminInvoicesPageSize);
      } else showError(res.data?.message || '更新开票状态失败');
    } catch (e) {
      showError('更新开票状态失败');
    } finally {
      setInvoiceUpdateSubmitting(false);
    }
  };
  const openProviderModal = (record) => setProviderModal({ visible: true, form: { provider_code: record.provider_code, name: record.name || PROVIDER_LABELS[record.provider_code] || record.provider_code, enabled: !!record.enabled, supported_payment_channels: record.supported_payment_channels || '[]', allow_cross_channel: !!record.allow_cross_channel, supports_create: !!record.supports_create, supports_red_invoice: !!record.supports_red_invoice, supports_query: !!record.supports_query, supports_download: !!record.supports_download, supports_email_forward: !!record.supports_email_forward, config: record.config || '{}' } });
  const submitProvider = async () => {
    setProviderSubmitting(true);
    try {
      const res = await API.put('/api/user/admin/invoice-provider-configs', providerModal.form);
      if (res.data?.success) {
        showSuccess('开票渠道配置已保存');
        setProviderModal({ visible: false, form: {} });
        loadProviders();
      } else showError(res.data?.message || '保存渠道配置失败');
    } catch (e) {
      showError('保存渠道配置失败');
    } finally {
      setProviderSubmitting(false);
    }
  };
  const topupColumns = [
    { title: '用户ID', dataIndex: 'user_id', width: 90 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    { title: '充值时间', dataIndex: 'create_time', width: 170, render: (value) => timestamp2string(value) },
    { title: '充值额度', dataIndex: 'amount', width: 120, render: (value) => (renderQuota ? renderQuota(value) : value) },
    { title: '支付金额', dataIndex: 'money', width: 110, render: (value) => <Text type='danger'>{formatMoney(value)}</Text> },
    { title: '支付方式', dataIndex: 'payment_method', width: 110, render: (value, record) => PAYMENT_METHOD_MAP[record.payment_channel] || PAYMENT_METHOD_MAP[value] || record.payment_channel || value || '-' },
    { title: '订单号', dataIndex: 'trade_no', render: (value) => <Text copyable={{ content: value }}>{value}</Text> },
    { title: '状态', dataIndex: 'status', width: 120, render: (_, record) => (record.invoice_status ? renderTag(INVOICE_STATUS, record.invoice_status) : <Tag color='green'>可开票</Tag>) },
    { title: '操作', width: 110, render: (_, record) => <Button size='small' type='primary' theme='outline' disabled={!record.invoice_available || !subjectVerified} onClick={() => openApplyModal([record])}>申请开票</Button> },
  ];
  const invoiceColumns = [
    { title: '申请ID', dataIndex: 'id', width: 90 },
    { title: '申请时间', dataIndex: 'created_at', width: 170, render: (value) => timestamp2string(value) },
    { title: '发票类型', dataIndex: 'invoice_type', width: 140, render: () => '增值税普通发票' },
    { title: '发票金额', dataIndex: 'amount', width: 120, render: (value) => <Text type='danger'>{formatMoney(value)}</Text> },
    { title: '状态', dataIndex: 'status', width: 120, render: (value) => renderTag(INVOICE_STATUS, value) },
    { title: '发票号', dataIndex: 'invoice_no', width: 160, render: (value) => value || '-' },
    { title: '操作', width: 160, render: (_, record) => <Space>{record.invoice_file_url ? <Button size='small' type='primary' theme='outline' onClick={() => window.open(record.invoice_file_url, '_blank', 'noopener,noreferrer')}>下载</Button> : null}{record.status === 'pending' ? <Button size='small' theme='outline' type='danger' onClick={() => cancelInvoice(record.id)}>取消</Button> : null}</Space> },
  ];
  const adminSubjectColumns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '用户ID', dataIndex: 'user_id', width: 90 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    { title: '类型', dataIndex: 'subject_type', width: 90, render: (value) => SUBJECT_LABELS[value] || value },
    { title: '主体', dataIndex: 'company_name', render: (_, record) => textOfSubject(record) },
    { title: '纳税号', dataIndex: 'tax_no', width: 180, render: (_, record) => taxNoOfSubject(record) },
    { title: '证件', dataIndex: 'masked_tax_no', render: (_, record) => docOfSubject(record) },
    { title: '有效期', dataIndex: 'valid_until', width: 170, render: (value) => formatValidUntil(value) },
    { title: '状态', dataIndex: 'status', width: 110, render: (value) => renderTag(SUBJECT_STATUS, value) },
    { title: '驳回原因', dataIndex: 'reject_reason', render: (value) => value || '-' },
    { title: '操作', width: 150, render: (_, record) => <Space><Button size='small' type='primary' theme='outline' onClick={() => openReviewModal(record, 'verified')}>通过</Button><Button size='small' type='danger' theme='outline' onClick={() => openReviewModal(record, 'rejected')}>驳回</Button></Space> },
  ];
  const adminInvoiceColumns = [
    { title: 'ID', dataIndex: 'id', width: 80 },
    { title: '用户ID', dataIndex: 'user_id', width: 90 },
    { title: '用户名', dataIndex: 'username', width: 120 },
    { title: '金额', dataIndex: 'amount', width: 110, render: (value) => <Text type='danger'>{formatMoney(value)}</Text> },
    { title: '支付渠道', dataIndex: 'payment_channel', width: 110, render: (value) => PAYMENT_METHOD_MAP[value] || value || '-' },
    { title: '状态', dataIndex: 'status', width: 110, render: (value) => renderTag(INVOICE_STATUS, value) },
    { title: '发票号', dataIndex: 'invoice_no', width: 150, render: (value) => value || '-' },
    { title: '文件', dataIndex: 'invoice_file_url', width: 100, render: (value) => (value ? <Button size='small' theme='borderless' onClick={() => window.open(value, '_blank', 'noopener,noreferrer')}>查看</Button> : '-') },
    { title: '创建时间', dataIndex: 'created_at', width: 170, render: (value) => timestamp2string(value) },
    { title: '操作', width: 90, render: (_, record) => <Button size='small' type='primary' theme='outline' onClick={() => openInvoiceUpdateModal(record)}>处理</Button> },
  ];
  const providerColumns = [
    { title: 'Provider', dataIndex: 'provider_code', width: 150 },
    { title: '名称', dataIndex: 'name', width: 160 },
    { title: '启用', dataIndex: 'enabled', width: 80, render: (value) => (value ? <Tag color='green'>启用</Tag> : <Tag color='grey'>关闭</Tag>) },
    { title: '支付渠道', dataIndex: 'supported_payment_channels', render: (value) => value || '[]' },
    { title: '交叉开票', dataIndex: 'allow_cross_channel', width: 100, render: (value) => (value ? '允许' : '不允许') },
    { title: '能力', width: 240, render: (_, record) => <Space wrap>{record.supports_create ? <Tag color='green'>开票</Tag> : null}{record.supports_red_invoice ? <Tag color='pink'>红冲</Tag> : null}{record.supports_query ? <Tag color='blue'>查询</Tag> : null}{record.supports_download ? <Tag color='cyan'>下载</Tag> : null}{record.supports_email_forward ? <Tag color='violet'>转发</Tag> : null}</Space> },
    { title: '操作', width: 90, render: (_, record) => <Button size='small' type='primary' theme='outline' onClick={() => openProviderModal(record)}>配置</Button> },
  ];

  const currentInvoiceRecord = invoiceUpdateModal.record || {};
  const currentInvoiceSubject = currentInvoiceRecord.subject || currentInvoiceRecord.subject_snapshot || {};
  const currentInvoiceTradeNos = (currentInvoiceRecord.items || []).map((item) => item.trade_no).filter(Boolean).join(' / ');

  if (!featureVisible) return null;

  return (
    <Card
      bordered={false}
      className='rounded-2xl shadow-sm'
      title={<div className='flex items-center gap-2'><FileText size={18} /><span>{t ? t('发票管理') : '发票管理'}</span>{!featureEnabled ? <Tag color='grey'>前端已关闭</Tag> : null}</div>}
      headerExtraContent={<Space>{rootUser ? <Space><span className='text-xs text-[var(--semi-color-text-1)]'>前端显示</span><Switch size='small' checked={featureEnabled} loading={featureSwitchLoading} onChange={(checked) => toggleFeatureEnabled(Boolean(checked))} /></Space> : null}<Button size='small' theme='borderless' loading={subjectLoading || topupsLoading || invoicesLoading || adminSubjectsLoading || adminInvoicesLoading || providersLoading} onClick={refreshActive}>刷新</Button></Space>}
    >
      {rootUser ? (
        <div className='mb-4 rounded-xl border border-[var(--semi-color-border)] p-3 bg-[var(--semi-color-fill-0)]'>
          <div className='mb-2 text-sm font-medium'>前端可见用户 ID</div>
          <Space align='start' wrap>
            <textarea
              className='min-h-[72px] w-[420px] max-w-full rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3 text-sm'
              placeholder='留空表示所有用户可见；多个用户 ID 可用逗号、空格或换行分隔'
              value={visibleUserIds}
              onChange={(e) => setVisibleUserIds(e.target.value)}
            />
            <Button type='primary' theme='outline' loading={visibleUserIdsSaving} onClick={saveVisibleUserIds}>保存可见用户</Button>
          </Space>
        </div>
      ) : null}
      <div className='mb-4 rounded-xl border border-dashed border-[var(--semi-color-border)] p-4 bg-[var(--semi-color-fill-0)]'>
        <div className='flex flex-col md:flex-row md:items-center md:justify-between gap-3'>
          <div className='flex items-start gap-3'>
            <ShieldCheck size={22} className='mt-1 text-[var(--semi-color-primary)]' />
            <div>
              <div className='flex items-center gap-2 flex-wrap'><Text strong>开票认证主体</Text>{subject ? renderTag(SUBJECT_STATUS, subject.status) : <Tag color='grey'>未认证</Tag>}{subject?.subject_type ? <Tag>{SUBJECT_LABELS[subject.subject_type]}</Tag> : null}</div>
              <div className='mt-2 text-sm text-[var(--semi-color-text-1)] space-y-1'>
                {subject ? <><div>抬头：{textOfSubject(subject)}</div><div>证件：{docOfSubject(subject)}</div><div>有效期至：{formatValidUntil(subject.valid_until)}</div>{subject.reject_reason ? <div className='text-red-500'>驳回原因：{subject.reject_reason}</div> : null}</> : <div>申请开票前需要先完成企业或个人实名认证。</div>}
              </div>
            </div>
          </div>
          <Button type='primary' theme='outline' disabled={subjectVerified} onClick={openSubjectModal}>{subject ? '更新认证' : '实名认证'}</Button>
        </div>
      </div>
      {!subjectVerified ? <Banner type='warning' className='mb-4' description='发票信息必须与实名认证主体一致。认证通过后，在证件有效期内不允许修改认证信息，也不能在企业/个人类型之间切换。' /> : null}
      <Tabs activeKey={activeKey} onChange={setActiveKey}>
        <Tabs.TabPane tab='充值记录' itemKey='topups'>
          <div className='mb-3 flex flex-col md:flex-row gap-3 md:items-center md:justify-between'>
            <Space wrap>
              <Input prefix={<IconSearch />} showClear placeholder='搜索订单号' value={topupsKeyword} onChange={setTopupsKeyword} className='md:max-w-sm' />
              {adminUser ? <Input showClear placeholder='用户ID，留空为自己' value={invoiceTargetUserId} onChange={setInvoiceTargetUserId} style={{ width: 180 }} /> : null}
            </Space>
            <Button type='primary' disabled={!subjectVerified || selectedRows.length === 0} onClick={() => openApplyModal(selectedRows)}>批量申请开票</Button>
          </div>
          <Table rowKey='id' size='small' columns={topupColumns} dataSource={topups} loading={topupsLoading} rowSelection={{ selectedRowKeys, onChange: (keys, rows) => { setSelectedRowKeys(keys); setSelectedRows(rows.filter((row) => row.invoice_available)); }, getCheckboxProps: (record) => ({ disabled: !record.invoice_available }) }} pagination={{ currentPage: topupsPage, pageSize: topupsPageSize, total: topupsTotal, showSizeChanger: true, onPageChange: setTopupsPage, onPageSizeChange: (size) => { setTopupsPageSize(size); setTopupsPage(1); } }} empty={<Empty description='暂无可展示充值记录' />} />
        </Tabs.TabPane>
        <Tabs.TabPane tab='开票记录' itemKey='invoices'>
          {adminUser ? <div className='mb-3 flex flex-col md:flex-row gap-3 md:items-center'><Input showClear placeholder='用户ID，留空为自己' value={invoiceTargetUserId} onChange={setInvoiceTargetUserId} style={{ width: 180 }} /></div> : null}
          <Table rowKey='id' size='small' columns={invoiceColumns} dataSource={invoices} loading={invoicesLoading} pagination={{ currentPage: invoicesPage, pageSize: invoicesPageSize, total: invoicesTotal, showSizeChanger: true, onPageChange: setInvoicesPage, onPageSizeChange: (size) => { setInvoicesPageSize(size); setInvoicesPage(1); } }} empty={<Empty description='暂无开票记录' />} />
        </Tabs.TabPane>
        {adminUser ? (
          <Tabs.TabPane tab='认证审核' itemKey='subject_admin'>
            <div className='mb-3 flex flex-col md:flex-row gap-3 md:items-center'>
              <select className='h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3' value={adminSubjectStatus} onChange={(e) => setAdminSubjectStatus(e.target.value)}><option value=''>全部状态</option><option value='pending'>待审核</option><option value='verified'>已认证</option><option value='rejected'>已驳回</option></select>
              <Input prefix={<IconSearch />} showClear placeholder='搜索用户/主体/证件' value={adminSubjectKeyword} onChange={setAdminSubjectKeyword} className='md:max-w-sm' />
            </div>
            <Table rowKey='id' size='small' columns={adminSubjectColumns} dataSource={adminSubjects} loading={adminSubjectsLoading} pagination={{ currentPage: adminSubjectsPage, pageSize: adminSubjectsPageSize, total: adminSubjectsTotal, showSizeChanger: true, onPageChange: setAdminSubjectsPage, onPageSizeChange: (size) => { setAdminSubjectsPageSize(size); setAdminSubjectsPage(1); } }} />
          </Tabs.TabPane>
        ) : null}
        {adminUser ? (
          <Tabs.TabPane tab='开票管理' itemKey='invoice_admin'>
            <div className='mb-3 flex flex-col md:flex-row gap-3 md:items-center'>
              <select className='h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3' value={adminInvoiceStatus} onChange={(e) => setAdminInvoiceStatus(e.target.value)}><option value=''>全部状态</option><option value='pending'>待处理</option><option value='approved'>已受理</option><option value='issued'>已开票</option><option value='rejected'>已驳回</option><option value='failed'>失败</option></select>
              <Input prefix={<IconSearch />} showClear placeholder='搜索订单/发票号/用户' value={adminInvoiceKeyword} onChange={setAdminInvoiceKeyword} className='md:max-w-sm' />
            </div>
            <Table rowKey='id' size='small' columns={adminInvoiceColumns} dataSource={adminInvoices} loading={adminInvoicesLoading} pagination={{ currentPage: adminInvoicesPage, pageSize: adminInvoicesPageSize, total: adminInvoicesTotal, showSizeChanger: true, onPageChange: setAdminInvoicesPage, onPageSizeChange: (size) => { setAdminInvoicesPageSize(size); setAdminInvoicesPage(1); } }} />
          </Tabs.TabPane>
        ) : null}
        {adminUser ? <Tabs.TabPane tab='渠道配置' itemKey='provider_admin'><Table rowKey='provider_code' size='small' columns={providerColumns} dataSource={providers} loading={providersLoading} pagination={false} /></Tabs.TabPane> : null}
      </Tabs>

      <Modal title='实名认证' visible={subjectModalVisible} onCancel={() => setSubjectModalVisible(false)} onOk={submitSubject} confirmLoading={subjectSubmitting} maskClosable={false}>
        <div className='space-y-4'>
          <div><Text strong>认证类型</Text><select className='mt-2 w-full h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3' value={subjectForm.subject_type} disabled={!!subject?.subject_type} onChange={(e) => setSubjectForm({ ...subjectForm, subject_type: e.target.value })}><option value='company'>企业实名认证</option><option value='personal'>个人实名认证</option></select></div>
          {subjectForm.subject_type === 'company' ? <><Input prefix='企业名称' value={subjectForm.company_name} onChange={(value) => setSubjectForm({ ...subjectForm, company_name: value })} /><Input prefix='纳税识别号' value={subjectForm.tax_no} onChange={(value) => setSubjectForm({ ...subjectForm, tax_no: value })} /></> : <><Input prefix='个人姓名' value={subjectForm.real_name} onChange={(value) => setSubjectForm({ ...subjectForm, real_name: value })} /><Input prefix='身份证号' value={subjectForm.id_no} onChange={(value) => setSubjectForm({ ...subjectForm, id_no: value })} /></>}
          <Input prefix='凭证链接' placeholder='营业执照/身份证认证材料链接，可选' value={subjectForm.certificate_url} onChange={(value) => setSubjectForm({ ...subjectForm, certificate_url: value })} />
          <div>
            <div className='mb-2 flex items-center justify-between'><Text strong>证件有效期</Text><Checkbox checked={subjectForm.valid_forever} onChange={(e) => setSubjectForm({ ...subjectForm, valid_forever: Boolean(e?.target?.checked), valid_until: e?.target?.checked ? '' : subjectForm.valid_until })}>长期有效</Checkbox></div>
            {!subjectForm.valid_forever ? <input type='date' className='w-full h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3' value={subjectForm.valid_until} onChange={(e) => setSubjectForm({ ...subjectForm, valid_until: e.target.value })} /> : null}
          </div>
        </div>
      </Modal>

      <Modal title='申请开票' visible={applyVisible} onCancel={() => setApplyVisible(false)} onOk={submitApply} confirmLoading={applySubmitting} maskClosable={false}>
        <div className='space-y-4'>
          <Banner type='info' description='发票类型为增值税普通发票；发票抬头、证件号/税号将使用已认证信息，不能手动填写或修改。' />
          <div className='grid grid-cols-2 gap-3 text-sm'><div>发票金额：{formatMoney(invoiceAmount)}</div><div>订单数量：{applyRows.length}</div><div>发票类型：增值税普通发票</div><div>主体类型：{SUBJECT_LABELS[subject?.subject_type] || '-'}</div><div className='col-span-2'>发票抬头：{textOfSubject(subject)}</div><div className='col-span-2'>认证证件：{docOfSubject(subject)}</div></div>
          <Input prefix='接收邮箱' placeholder='用于接收电子发票' value={applyEmail} onChange={setApplyEmail} />
        </div>
      </Modal>

      <Modal title={reviewModal.status === 'verified' ? '通过认证' : '驳回认证'} visible={reviewModal.visible} onCancel={() => setReviewModal({ visible: false, record: null, status: 'verified', reason: '' })} onOk={submitReview} confirmLoading={reviewSubmitting} maskClosable={false}>
        <div className='space-y-3'><div>主体：{textOfSubject(reviewModal.record)}</div><div>类型：{SUBJECT_LABELS[reviewModal.record?.subject_type] || '-'}</div><textarea className='w-full min-h-[90px] rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3' placeholder='驳回原因，通过时可留空' value={reviewModal.reason} onChange={(e) => setReviewModal({ ...reviewModal, reason: e.target.value })} /></div>
      </Modal>

      <Modal title='处理开票申请' visible={invoiceUpdateModal.visible} onCancel={() => setInvoiceUpdateModal({ visible: false, record: null, form: {} })} onOk={submitInvoiceUpdate} confirmLoading={invoiceUpdateSubmitting} maskClosable={false}>
        <div className='space-y-4'>
          <div className='rounded-xl border border-[var(--semi-color-border)] bg-[var(--semi-color-fill-0)] p-3 text-sm grid grid-cols-1 md:grid-cols-2 gap-2'>
            <div>用户：{currentInvoiceRecord.username || '-'}（ID {currentInvoiceRecord.user_id || '-'}）</div>
            <div>发票金额：{formatMoney(currentInvoiceRecord.amount)}</div>
            <div>发票抬头：{textOfSubject(currentInvoiceSubject)}</div>
            <div>主体类型：{SUBJECT_LABELS[currentInvoiceSubject?.subject_type] || '-'}</div>
            <div className='md:col-span-2'>纳税号：{taxNoOfSubject(currentInvoiceSubject)}</div>
            <div className='md:col-span-2'>认证证件：{docOfSubject(currentInvoiceSubject)}</div>
            <div className='md:col-span-2'>接收邮箱：{currentInvoiceRecord.email || '-'}</div>
            <div className='md:col-span-2'>订单号：{currentInvoiceTradeNos || '-'}</div>
          </div>
          <select className='w-full h-9 rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] px-3' value={invoiceUpdateModal.form.status || 'approved'} onChange={(e) => setInvoiceUpdateModal({ ...invoiceUpdateModal, form: { ...invoiceUpdateModal.form, status: e.target.value } })}><option value='approved'>已受理</option><option value='issued'>已开票</option><option value='rejected'>已驳回</option><option value='failed'>开票失败</option><option value='red_pending'>红冲处理中</option><option value='red_issued'>已红冲</option></select>
          <Input prefix='发票号' value={invoiceUpdateModal.form.invoice_no || ''} onChange={(value) => setInvoiceUpdateModal({ ...invoiceUpdateModal, form: { ...invoiceUpdateModal.form, invoice_no: value } })} />
          <Input prefix='文件链接' value={invoiceUpdateModal.form.invoice_file_url || ''} onChange={(value) => setInvoiceUpdateModal({ ...invoiceUpdateModal, form: { ...invoiceUpdateModal.form, invoice_file_url: value } })} />
          <Input prefix='文件类型' value={invoiceUpdateModal.form.file_type || 'pdf'} onChange={(value) => setInvoiceUpdateModal({ ...invoiceUpdateModal, form: { ...invoiceUpdateModal.form, file_type: value } })} />
          {invoiceUpdateModal.form.status === 'issued' ? <Checkbox checked={invoiceUpdateModal.form.send_email !== false} onChange={(e) => setInvoiceUpdateModal({ ...invoiceUpdateModal, form: { ...invoiceUpdateModal.form, send_email: Boolean(e?.target?.checked) } })}>推送邮箱</Checkbox> : null}
          <textarea className='w-full min-h-[90px] rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3' placeholder='驳回/失败原因' value={invoiceUpdateModal.form.reject_reason || ''} onChange={(e) => setInvoiceUpdateModal({ ...invoiceUpdateModal, form: { ...invoiceUpdateModal.form, reject_reason: e.target.value } })} />
        </div>
      </Modal>

      <Modal title='开票渠道配置' visible={providerModal.visible} onCancel={() => setProviderModal({ visible: false, form: {} })} onOk={submitProvider} confirmLoading={providerSubmitting} maskClosable={false} width={720}>
        <div className='space-y-4'>
          <Input prefix='Provider' disabled value={providerModal.form.provider_code || ''} />
          <Input prefix='名称' value={providerModal.form.name || ''} onChange={(value) => setProviderModal({ ...providerModal, form: { ...providerModal.form, name: value } })} />
          <div className='grid grid-cols-2 gap-3 text-sm'>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.enabled} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, enabled: Boolean(checked) } })} />启用渠道</label>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.allow_cross_channel} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, allow_cross_channel: Boolean(checked) } })} />允许交叉开票</label>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.supports_create} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, supports_create: Boolean(checked) } })} />支持开票</label>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.supports_red_invoice} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, supports_red_invoice: Boolean(checked) } })} />支持红冲</label>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.supports_query} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, supports_query: Boolean(checked) } })} />支持查询</label>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.supports_download} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, supports_download: Boolean(checked) } })} />支持下载</label>
            <label className='flex items-center gap-2'><Switch size='small' checked={!!providerModal.form.supports_email_forward} onChange={(checked) => setProviderModal({ ...providerModal, form: { ...providerModal.form, supports_email_forward: Boolean(checked) } })} />支持邮件转发</label>
          </div>
          <div><Text strong>支持支付渠道 JSON</Text><textarea className='mt-2 w-full min-h-[80px] rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3 font-mono text-xs' value={providerModal.form.supported_payment_channels || '[]'} onChange={(e) => setProviderModal({ ...providerModal, form: { ...providerModal.form, supported_payment_channels: e.target.value } })} /></div>
          <div><Text strong>第三方配置 JSON</Text><textarea className='mt-2 w-full min-h-[120px] rounded-md border border-[var(--semi-color-border)] bg-[var(--semi-color-bg-0)] p-3 font-mono text-xs' value={providerModal.form.config || '{}'} onChange={(e) => setProviderModal({ ...providerModal, form: { ...providerModal.form, config: e.target.value } })} /></div>
        </div>
      </Modal>
    </Card>
  );
};

export default InvoiceWalletCard;
