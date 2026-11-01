# 服务流量与 WireGuard 峰值：后端交接

最后更新：2026-11-01

本次交付后端采集、存储、接口和测试。前端由另一位 Agent 接入；用户人工验收通过后，另行构建、打包并发布新版本。本阶段不发布 Release、不更新线上容器。

## 统计口径

服务调用统计两段路径的四个方向，方向均以本站为参照：

| 字段 | 含义 |
| --- | --- |
| `client_in` | 客户端 → 本站，实际被应用读取的请求正文或 WebSocket 消息 |
| `client_out` | 本站 → 客户端，实际被响应写入接口接受的正文或消息 |
| `upstream_out` | 本站 → 模型上游，以及该服务调用触发的图片、搜索等上游请求正文 |
| `upstream_in` | 上游 → 本站，实际被应用读取的响应正文或消息 |
| `in` | `client_in + upstream_in` |
| `out` | `client_out + upstream_out` |
| `total` | 四个方向之和，等于 `in + out` |

同一内容从上游传到本站，再从本站传到客户端，会在两个方向各记一次。这符合观察本站双向流量的需求。**服务流量不能再与 WG 流量相加**：走 WG 的上游服务流量已经包含在两者中，二者观察层级不同。

- 采集挂在服务网关路由上，包含模型列表、令牌估算、流式输出、WebSocket、图片任务及该路由上的失败响应；不以“请求成功”作为计数条件。
- HTTP 在读取/写入时累计，不等请求结束；重试、回退产生的实际正文会再次累计。未被应用读取的请求剩余部分不计入。
- 覆盖通用 HTTP 上游、账号路由传输、WS 连接池及预热、实时语音、异步/批量图片、业务图片下载及搜索客户端。后台独立巡检、版本检查、管理后台和用户面板、数据库和 Redis 通信不计入。业务请求内触发的附属请求只有经过上述受观测传输路径才纳入；独立插件自行建立的网络连接不在保证范围内。
- `service_payload_v1` 是**应用可见载荷**口径，不是网卡或云厂商账单。HTTP 按传输组件暴露的 Body 字节计数：标准 Go 自动解压 gzip 时计解压后字节，调用方显式处理压缩响应时计暴露的压缩正文。WS 计解压后的消息载荷，不计控制帧。HTTP 头、TLS、TCP/IP、重传、代理隧道封装等未纳入。写入成功不等于远端已确认收齐。
- WG 继续使用 `wg_udp_payload_v1`：应用 UDP socket 收发的加密 UDP 载荷，含握手、保活、隧道内 DNS/探测和重传；不含外层 IP/UDP 头及其他进程流量。它也不是完整服务器账单。
- 因此大卡片建议标注“服务流量（应用载荷）”；带宽峰值用于判断业务负载量级，不能标为“服务器实际峰值带宽”或精确费用。

## 1 秒峰值的定义

后台每秒采样，速率 = 两次采样间新增字节 / 实际经过秒数。并发连接先汇总再计算；总速率峰值取同一次采样四方向之和的最大值，**不得把入站、出站各自的历史峰值相加**。

每个峰值附带采样结束时间 `at` 和实际窗口长度 `window_seconds`。目标为 1 秒，正常调度误差接受 0.5–1.5 秒；超出此范围的累计字节仍保留，但该样本不参与峰值，并增加 `invalid_samples`。停止进程时的不足一秒尾段也遵循此规则。采样与数据库写入分开；若进程暂停或调度阻塞，明确返回异常状态，不伪装成准确的 1 秒峰值。

这是 **1 秒平均峰值**，不能反映毫秒级突发或网络接口实际瞬时速率。峰值时间是采样结束时间，不是某个数据包的准确到达时间。跨分钟/小时的样本归入结束时间所在桶，边界存在一个样本的误差。

采集不依赖页面打开或刷新；运维监控开关控制接口访问，不关闭后台采集。当前速率约每秒更新；历史及区间汇总约每 5 秒保存一次。分钟数据保留 7 天，小时数据保留 90 天；小时峰值保留原始有效样本最大值，不用小时平均速率替代。

## 服务流量 API

`GET /api/v1/admin/ops/service-traffic`

沿用管理后台认证，仅完整管理员可访问；受限管理员凭据返回 403。监控关闭返回 404；未初始化返回 503；读取失败返回 500。查询仅接受 `range`、`interval`、`start`、`end`。不要携带运维页面的 `group_id`、`platform` 等其他筛选参数，否则返回 400；这是当前数据库内的整实例历史，不支持按账号或分组拆分。

| 参数 | 取值和默认值 |
| --- | --- |
| `range` | `24h`（默认）、`7d`、`30d`、`90d`、`custom` |
| `interval` | `minute` 或 `hour`；24h 默认 minute，其他范围默认 hour |
| `start` | 仅 custom 必填；带时区的 RFC3339 时间 |
| `end` | 仅 custom 可填；省略代表截至当前时间 |

例如 `?range=24h`、`?range=7d&interval=hour`。自定义查询的 start 必须对齐 UTC 分钟/小时；过去的 end 同样必须对齐。结束时间在未来不超过 25 小时时截断到现在并返回 `end_clamped=true`。按半开区间 `[start,end)` 查询；预设窗口舍弃最早的不完整桶，以响应 `start/end` 为实际范围。当前未结束的桶仅含已保存数据。

错误原因 `reason`：`TRAFFIC_INTERVAL_INVALID`、`TRAFFIC_DATE_INVALID`、`TRAFFIC_BOUNDARY_UNALIGNED`、`TRAFFIC_RANGE_INVALID`、`TRAFFIC_RETENTION_EXCEEDED`。如 `range=30d&interval=minute` 超过分钟保留范围，返回 400，不静默换成小时。

响应外层沿用 `{ "code": 0, "message": "success", "data": ... }`。以下 TypeScript 仅作为接口约定，前端自行接入：

```ts
type Direction = 'client_in' | 'client_out' | 'upstream_out' | 'upstream_in'
type RateKey = Direction | 'in' | 'out' | 'total'
type Bytes = Record<Direction, string> // 十进制整数字符串，避免 JS 大整数精度丢失
type Peak = {
  bytes_per_second: number // B/s，浮点数
  at: string              // RFC3339，采样结束时间
  window_seconds: number
}
type Quality = {
  first_sample_at: string | null
  last_sample_at: string | null
  valid_samples: number
  invalid_samples: number
}
type TrafficWindow = {
  range: '24h' | '7d' | '30d' | '90d' | 'custom'
  interval: 'minute' | 'hour'
  start: string
  end: string
  requested_start: string
  requested_end: string
  retained_from: string
  as_of: string
  end_clamped: boolean
}
type ServiceBucket = {
  start: string
  interval: 'minute' | 'hour'
  bytes: Bytes
  peaks: Partial<Record<RateKey, Peak>> | null
  quality: Quality
}
type ServiceTrafficData = {
  report: TrafficWindow & {
    metric_basis: 'service_payload_v1'
    scope: 'single_instance_history'
    peak_window_seconds: 1
    bytes: Bytes
    in_bytes: string
    out_bytes: string
    total_bytes: string
    peaks: Partial<Record<RateKey, Peak>> | null
    quality: Quality
    history: ServiceBucket[] // 按 start 升序，不自动补空桶
    has_records: boolean
    incomplete: boolean
    concurrent_runtimes: boolean
    saved_at: string | null
  }
  current: {
    bytes_per_second: Partial<Record<RateKey, number>>
    sampled_at: string
    window_seconds: number
    state: 'warming_up' | 'sampled' | 'invalid_interval'
  }
  last_persisted_at: string | null
  persistence_error: boolean
}
```

- 区间大卡片读取 `report.in_bytes/out_bytes/total_bytes`，峰值读取 `report.peaks.in/out/total`；当前速率读取 `current`。当前速率总是当前进程最新采样，**不随历史范围改变**。
- 字节总数用字符串或 BigInt 处理；最终格式化时再换算 KiB/MiB/GiB。速率可展示 B/s、KiB/s、MiB/s；若展示带宽 Mbps，公式为 `bytes_per_second * 8 / 1_000_000`。不要把 MB/s 与 Mbps 混用。
- `peaks=null` 或缺少某个 key 表示该峰值未知；有效样本 `bytes_per_second=0` 才表示观察到了零速率。空记录 `has_records=false`、`history=[]`，不能把它解读为全时段无流量。
- `quality` 给出观察到的首末样本及有效/异常样本数；它不是完整连续覆盖证明。安装之前、停机期间和丢失的尾部样本无法补算。新旧历史混合时，不要声称峰值覆盖整个所选期间。
- `incomplete=true` 表示异常采样、未正常结束的旧进程、未保存明细丢失或多个实例重叠等情形。`persistence_error=true` 表示最近保存失败；`saved_at` 是查询范围内记录所属运行的最新保存采样时间，`last_persisted_at` 是当前进程最近成功保存的采样时间。
- `current.state=warming_up` 时 `sampled_at` 为 Go 零值时间，窗口为 0、速率为空；前端显示等待采样。`invalid_interval` 显示采样延迟，不当作可信 1 秒速率；按 `sampled_at` 检查陈旧数据。
- 历史跨重启合并，但不提供多副本集群峰值：发现同库多个进程运行时间重叠时，`concurrent_runtimes=true`、`incomplete=true`，总量仍累计，区间及历史桶峰值置 null；`current` 仍仅属请求命中的进程。

## WG 历史流量 API 的增量

继续使用 `GET /api/v1/admin/proxies/:id/wireguard/traffic`，相同范围参数、原有认证方式和 totals/history 字段保持兼容。新增：

```ts
type WGPeakHistory = {
  peaks: Partial<Record<'tx' | 'rx' | 'total', Peak>> | null
  quality: Quality
}
// data 中新增：
// peak_window_seconds: 1
// peak_history: WGPeakHistory | null
// data.history 每个元素新增：
// peak_history: WGPeakHistory | null
```

`tx` 为本站 WG 发送，`rx` 为本站 WG 接收，`total` 为同一采样内 tx+rx 的峰值。24 小时峰值直接读取 `data.peak_history.peaks.tx/rx`，不要从曲线平均速率反推。保留原 `tx_bytes/rx_bytes/total_bytes` 十进制字符串。

节点详情 `GET /api/v1/admin/proxies/:id/wireguard` 的 `rate` 仍有 `tx_bytes_per_second/rx_bytes_per_second/window_seconds/sampled_at/state`，但原约 5 秒平均改为约 1 秒平均。`state` 包括 `warming_up`、`sampled`、`invalid_interval`、`not_running`；异常计数状态也须按不可用处理。详情接口附带的旧 `history` 不提供峰值，历史卡片应使用 `/traffic` 接口。

升级前的 WG 明细 `peak_history=null`，旧累计字节保留。新样本发生后才有峰值；混合时段通过 `quality.first_sample_at/last_sample_at` 展示峰值的实际观察范围。查询中的 `incomplete` 继续反映节点历史不完整，并增加异常采样提示。

## 持久化与恢复

新增迁移 `253_service_traffic_peaks.sql`，仅新增 WG 峰值列及服务流量表；没有修改 250–252 或更早迁移，没有重置旧 WG 总量或节点配置。无需新增服务、端口或生产配置项。

每次保存有运行标识和递增序号，同一快照重复保存、旧快照晚到不会重复累计。正常关闭在数据库关闭前尽力保存尾部。异常强杀可能丢失最近一次成功保存之后的数据；缓存积压被限定在有限时段，超出时明确标记不完整。保存失败不会为业务请求引入同步数据库写入。

这套统计用于运维观察，不修改用量账单、模型 token 费用或用户余额扣费。

## 前端交付与验收项

1. 运维页增加整实例服务流量卡片：选定期间的入站、出站、合计，入站/出站/合计 1 秒平均峰值及时间，当前速率和观察范围。不套用分组/供应商筛选。
2. WG 历史卡片展示所选期间发送、接收 1 秒平均峰值及时间；保留原有累计量和图表。
3. 单位、零值、未知、升级前历史、无数据、采样延迟、保存失败、部分历史及并发实例各状态均可辨认；切换范围应更新历史峰值，不能沿用前一个范围的数据。
4. 连续 SSE/WS 尚未结束时，速率应变化；管理后台刷新本身不增加服务流量。发起两个并发调用时观察汇总值，累计量与峰值不得混淆。
5. 重启测试实例后历史保持；同一数据库迁移重复执行不损坏旧 WG 累计。生产验收前请使用隔离测试数据库及独立 WG Peer。
6. 完成前端接入测试后交给用户人工验收。仅在用户确认后进入正式发布，再验证后台检测更新和一键重启链路。

## 测试与运行环境

后端测试入口：

```sh
cd backend
make test-unit
golangci-lint run --timeout=30m
go test -race ./internal/pkg/traffic ./internal/pkg/wgtunnel/... ./internal/pkg/egress/... ./internal/server/middleware -count=1
# 使用隔离数据库的 DSN；不可设置为生产数据库。
SUB2API_WG_TEST_DSN='<isolated PostgreSQL DSN>' CI=true go test -tags=integration ./internal/repository -run 'Test(ServiceTraffic|WireGuard|MigrationsRunner)' -count=1 -v -timeout=240s
# 远端测试需要独立 Peer 配置文件和受控 fixture，不可指向生产模型接口。
SUB2API_WG_BANDWIDTH_TEST_CONFIG='<private config path>' SUB2API_WG_BANDWIDTH_TEST_URL='<private fixture URL>' go test ./internal/pkg/wgtunnel -run '^TestAuthorizedRemoteBandwidth$' -count=1 -v
```

远端 fixture 为每方向 4 MiB 合成传输，用于验证真实 WG 收发计数及非零 1 秒峰值，不用于测试链路最大带宽。测试参数来源为用户在当前对话提供的授权连接信息；文档不保存连接地址、私钥或凭据。

当前测试 WG 资源按用户要求保留到人工验收后再清理。独立接口及 fixture 不配置默认路由、NAT 或防火墙，也不复用生产 Peer；只能访问合成传输 fixture，不代表真实模型供应商出口已验收。它们是临时运行资源，服务器重启后不保证恢复。本地测试配置也只放在受限临时目录；前端 Agent 如需完整应用联调，需另行启动隔离应用环境，不能把这些核心测试当作前后端已验收。

### 本次后端验证结果

- `make test-unit` 全量通过；期间将进程总内存断言改为独立子进程测量，原 20000 次负载和 8 MiB 增量阈值保留。该阈值仅用于测试，不限制生产内存或流量。
- `golangci-lint run --timeout=30m`：0 issues。
- traffic、WG/netstack、egress、流量中间件的 race 检查通过；流式中途采样、压缩正文口径、WS 连接后台读写与失败握手有对应测试。
- 真实 PostgreSQL 集成测试：ServiceTraffic/WireGuard/MigrationsRunner 共 23 个顶层用例通过，涵盖重复保存、重启、并发运行识别、保留期限、旧版本升级及迁移重复执行。
- Linux amd64 后端编译通过；镜像更新器现有 14 项 Python 测试通过。
- macOS 及 Ubuntu 24.04 arm64 容器完成授权 WG 合成双向传输。最终 Linux 复测应用上传/下载各 4 MiB，WG 发送 4738932 B、接收 4517724 B，发送/接收峰值约 2658525/2308377 B/s。该速率仅为本次合成负载观测值，不是链路极限。
- 未执行前端构建、完整应用的真实供应商联调或用户验收，未推送、发布或升级生产服务。
