import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';
import {
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Layout,
  Popover,
  Result,
  Row,
  Spin,
  Statistic,
  Table,
  Tag,
  Tooltip,
} from 'antd';
import type { TableColumnsType } from 'antd';
import { CloudServerOutlined, LinkOutlined, ReloadOutlined, TeamOutlined } from '@ant-design/icons';

import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { usePageTitle } from '@/hooks/usePageTitle';
import { useDatepicker } from '@/hooks/useDatepicker';
import { useWebSocket } from '@/hooks/useWebSocket';
import AppSidebar from '@/layouts/AppSidebar';
import { keys } from '@/api/queryKeys';
import { parseMsg } from '@/utils/zodValidate';
import { HttpUtil, IntlUtil } from '@/utils';
import {
  OnlineClientViewSchema,
  type OnlineClientView,
  type OnlineClientIP,
} from '@/generated/zod';

const OnlineClientListSchema = z.array(OnlineClientViewSchema);

const PROTOCOL_COLORS: Record<string, string> = {
  vless: 'blue',
  vmess: 'geekblue',
  trojan: 'volcano',
  shadowsocks: 'magenta',
  hysteria: 'cyan',
  hysteria2: 'green',
  wireguard: 'gold',
  amneziawg: 'yellow',
  http: 'purple',
  mixed: 'lime',
  tunnel: 'orange',
};

// Chips shown inline per row; the rest collapse into a "+N more" popover so a
// heavy client does not stretch the table into an endless tag wall.
const IPS_CHIP_LIMIT = 5;

async function fetchOnlineClientsView(): Promise<OnlineClientView[]> {
  const msg = await HttpUtil.post('/panel/api/clients/onlineClients', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch online clients');
  const validated = parseMsg(msg, OnlineClientListSchema, 'clients/onlineClients');
  return validated.obj ?? [];
}

export default function OnlineClientsPage() {
  usePageTitle();
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const { datepicker } = useDatepicker();

  const query = useQuery({
    queryKey: keys.clients.onlineClients(),
    queryFn: fetchOnlineClientsView,
    staleTime: Infinity,
  });
  // The panel pushes a "traffic" event every poll tick; refetching on it keeps
  // this page live without its own timer.
  useWebSocket({ traffic: () => void query.refetch() });

  const rows = query.data ?? [];
  const loading = query.isFetching;
  const fetched = query.data !== undefined || query.isError;
  const fetchError = query.error ? (query.error as Error).message : '';

  let localCount = 0;
  let nodeCount = 0;
  for (const row of rows) {
    if (row.node) nodeCount += 1;
    else localCount += 1;
  }
  const counts = { total: rows.length, local: localCount, nodes: nodeCount };

  const dateLabel = (ts: number) => (!ts || ts <= 0 ? '-' : IntlUtil.formatDate(ts, datepicker));

  const ipChip = (ip: OnlineClientIP, key: string | number) => (
    <Tooltip key={key} title={`${t('pages.online.lastSeen')}: ${dateLabel(ip.timestamp)}`}>
      <Tag
        color="blue"
        style={{
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
          marginBottom: 4,
        }}
      >
        {ip.ip}
      </Tag>
    </Tooltip>
  );

  const columns: TableColumnsType<OnlineClientView> = [
    {
      title: t('pages.clients.email'),
      dataIndex: 'email',
      key: 'email',
      width: 220,
      ellipsis: true,
      render: (email: string) => <Tag color="green">{email}</Tag>,
    },
    {
      title: t('comment'),
      dataIndex: 'remark',
      key: 'remark',
      width: 180,
      ellipsis: true,
      render: (remark: string) => (remark ? remark : <span className="text-secondary">-</span>),
    },
    {
      title: t('pages.inbounds.remark'),
      dataIndex: 'inbound',
      key: 'inbound',
      width: 180,
      ellipsis: true,
      render: (_: unknown, row: OnlineClientView) => {
        if (!row.inbound) return '-';
        const color = PROTOCOL_COLORS[row.protocol.toLowerCase()] ?? 'default';
        return (
          <span>
            {row.inbound}
            {row.protocol ? (
              <Tag color={color} style={{ marginInlineStart: 6 }}>
                {row.protocol}
              </Tag>
            ) : null}
          </span>
        );
      },
    },
    {
      title: t('pages.online.node'),
      dataIndex: 'node',
      key: 'node',
      width: 160,
      ellipsis: true,
      render: (node: string) => (node ? <Tag color="geekblue">{node}</Tag> : '-'),
    },
    {
      title: t('pages.online.ips'),
      dataIndex: 'ips',
      key: 'ips',
      render: (_: unknown, row: OnlineClientView) => {
        const ips = row.ips ?? [];
        if (ips.length === 0) return <span className="text-secondary">-</span>;
        const visible = ips.slice(0, IPS_CHIP_LIMIT);
        const overflow = ips.slice(IPS_CHIP_LIMIT);
        return (
          <span>
            {visible.map((ip, idx) => ipChip(ip, `${row.email}-${idx}`))}
            {overflow.length > 0 && (
              <Popover
                trigger="click"
                placement="bottomRight"
                content={
                  <div style={{ maxWidth: 360, maxHeight: 280, overflowY: 'auto' }}>
                    {ips.map((ip, idx) => ipChip(ip, idx))}
                  </div>
                }
              >
                <Tag color="default" style={{ marginBottom: 4 }}>
                  +{overflow.length} {t('more')}
                </Tag>
              </Popover>
            )}
          </span>
        );
      },
    },
  ];

  const pageClass = useMemo(() => {
    const classes = ['online-clients-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" loading={loading} onClick={() => void query.refetch()}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
                  <Col span={24}>
                    <Card
                      size="small"
                      hoverable
                      className="summary-card"
                      title={t('menu.onlineClients')}
                      extra={
                        <Button
                          size="small"
                          icon={<ReloadOutlined />}
                          loading={loading}
                          onClick={() => void query.refetch()}
                        >
                          {t('refresh')}
                        </Button>
                      }
                    >
                      <Row gutter={[16, isMobile ? 16 : 12]}>
                        <Col xs={12} sm={12} md={8}>
                          <Statistic
                            title={t('pages.online.totalClients')}
                            value={String(counts.total)}
                            prefix={<TeamOutlined />}
                          />
                        </Col>
                        <Col xs={12} sm={12} md={8}>
                          <Statistic
                            title={t('pages.online.localClients')}
                            value={String(counts.local)}
                            prefix={<LinkOutlined />}
                          />
                        </Col>
                        <Col xs={12} sm={12} md={8}>
                          <Statistic
                            title={t('pages.online.nodeClients')}
                            value={String(counts.nodes)}
                            prefix={<CloudServerOutlined />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col span={24}>
                    <Card size="small">
                      <Table<OnlineClientView>
                        rowKey={(row) => `${row.node}\u0000${row.email}`}
                        columns={columns}
                        dataSource={rows}
                        loading={loading}
                        size={isMobile ? 'small' : 'middle'}
                        pagination={{ pageSize: 20, showSizeChanger: true }}
                        locale={{ emptyText: <Empty description={t('pages.online.noClients')} /> }}
                        scroll={{ x: 900 }}
                      />
                    </Card>
                  </Col>
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
