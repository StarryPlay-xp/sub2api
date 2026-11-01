# 服务流量出口 / 上游账号归因：第二期后端交接

最后更新：2026-11-01

第二期在 `service_payload_v1` 中补充上游出口与上游账号维度。统计的只是本站与上游之间的应用载荷，客户端两方向不进入出口统计。与[第一期用户 / API Key 归因](SERVICE_TRAFFIC_ATTRIBUTION_HANDOFF.md)共用采样器与保存事务。本轮仅后端和文档，不变更前端、版本、线上服务或本地验收应用。

## 接口

- `GET /api/v1/admin/ops/traffic-egress`
- `GET /api/v1/admin/ops/traffic-egress/:route_type/:route_id`，直连使用 `direct/0`。

两个接口均在现有管理员认证中间件之后执行：完整管理员可用，受限管理员 403，运维监控关闭 404，采样器或出口仓储未初始化 503，数据库失败 500。响应外层为 `{code,message,data}`，仅成功时返回下述 data。

只支持 `range/start/end/interval/compare/timezone`。默认 24h 和 hour；range 支持 24h/7d/30d/90d/custom，interval 只能 hour，compare 只能 previous。timezone 接受并忽略。未知参数、重复参数（包括重复 timezone）返回 400。窗口规则沿用一期：历史 custom 边界对齐 UTC 整点，省略 end 或合法未来 end 截到当前时刻，未来 end 最多 25 小时。历史不补空桶，数组为空时返回 `[]`。

字节和有符号增长量为十进制字符串，B/s 速率和 share 为数值；share 分母为 0 时 null。出口 share 分母为当前窗口全部已识别加未识别上游字节，账号 share 分母为当前出口字节。account_count 只计当前窗口有非零字节的真实账号，不含残余账号和仅上一时段有流量的账号。名称是当前身份表名称，不是请求发生时的历史快照。软删除保留名称和删除标记；硬删除只保留 ID 和删除标记（出口名称 null，账号名称/平台/类型为空字符串）。

```ts
type UpstreamBytes = { upstream_out: string; upstream_in: string }
type Peak = { bytes_per_second: number; at: string; window_seconds: number }
type EgressPeaks = Partial<Record<'out' | 'in' | 'total', Peak>> | null // out=本站→上游，in=上游→本站
type EgressRouteType = 'direct' | 'proxy' | 'wireguard'

type EgressRouteIdentity = {
  route_type: EgressRouteType
  route_id: number | null      // direct 为 null；proxy / wireguard 为 proxies.id
  route_name: string | null    // 代理或节点名称；direct 为 null
  protocol: string | null      // proxies.protocol
  route_deleted: boolean       // 已软删除的代理仍返回名称并标记
}

type EgressRouteStats = EgressRouteIdentity & {
  bytes: UpstreamBytes
  total_bytes: string
  share: number | null         // 占全部上游流量（含未识别出口）的比例，0–1
  attempts: number             // 经此出口的上游尝试次数，口径请在文档写明
  peaks: EgressPeaks
  account_count: number        // 窗口内经此出口有流量的上游账号数
  // 仅 wireguard：同一时间窗口（相同桶边界）的 WG 隧道字节，供前端计算隧道额外占用
  wireguard: { tx_bytes: string; rx_bytes: string; total_bytes: string; has_records: boolean } | null
  previous: { total_bytes: string } | null   // 仅 compare=previous
  growth_bytes: string | null                // 有符号十进制字符串，仅 compare=previous
}

type EgressAccountStats = {
  account_id: number
  account_name: string
  platform: string
  account_type: string
  account_deleted: boolean
  bytes: UpstreamBytes
  total_bytes: string
  share: number | null         // 占该出口流量的比例
  attempts: number
  peaks: EgressPeaks
  previous: { total_bytes: string } | null
  growth_bytes: string | null
}

// TrafficWindow 同第一期（range/interval/start/end/requested_start/requested_end/retained_from/as_of/end_clamped）
type EgressWindow = TrafficWindow & {
  metric_basis: 'service_payload_v1'
  egress_started_at: string | null  // 最早的按出口统计样本时间
  has_records: boolean
  incomplete: boolean
  concurrent_runtimes: boolean
  overflow: boolean
}
type PreviousWindow = { start: string; end: string; complete: boolean } | null

// 1) GET /api/v1/admin/ops/traffic-egress
// 参数：range、start、end、interval（仅 hour）、compare（仅 previous）
type TrafficEgressData = {
  window: EgressWindow
  previous_window: PreviousWindow
  routes: EgressRouteStats[]   // 当前或上一时段有字节/attempts 的出口；按当前 total_bytes 降序，再 type/id 升序
  totals: {
    upstream: { bytes: UpstreamBytes; total_bytes: string }      // 与 service-traffic 上游两方向同口径
    unidentified: { bytes: UpstreamBytes; total_bytes: string }  // 无法识别出口的上游流量
  }
}

// 2) GET /api/v1/admin/ops/traffic-egress/:route_type/:route_id    direct 的 route_id 用 0
// 参数：range、start、end、interval（仅 hour）、compare（仅 previous）
type TrafficEgressDetailData = {
  window: EgressWindow
  previous_window: PreviousWindow
  summary: EgressRouteStats    // 窗口内无数据时字节为 "0"、peaks 为 null
  history: Array<{ start: string; interval: 'hour'; bytes: UpstreamBytes; attempts: number; peaks: EgressPeaks }>
  accounts: EgressAccountStats[] // 当前或上一时段有字节/attempts 的真实账号；按当前 total_bytes 降序，再 id 升序
  unattributed_account: {
    bytes: UpstreamBytes
    total_bytes: string
    attempts: number
  } // 当前出口中缺失账号或超限后的残余；不放进 accounts
}
```

`TrafficWindow` 完全复用一期字段与类型：range / interval / start / end / requested_start / requested_end / retained_from / as_of / end_clamped。`EgressWindow` 与整个查询窗口相关；详情页的 has_records/overflow/incomplete 也反映全实例窗口，不是仅此出口。

### 错误原因

- `TRAFFIC_ROUTE_INVALID`（400）：类型不在 direct/proxy/wireguard 中，ID 解析失败或溢出、proxy/wireguard 非正整数、direct 非 0。
- `TRAFFIC_ROUTE_NOT_FOUND`（404）：对应类型的代理/节点不在身份表，也没有仍保留的历史；direct/0 始终合法。现有代理/节点或有历史的已删除出口，窗口无数据时可返回零值详情。
- `TRAFFIC_FILTER_INVALID`（400）：未知/重复参数或非法 compare。
- 沿用 `TRAFFIC_INTERVAL_INVALID`、`TRAFFIC_DATE_INVALID`、`TRAFFIC_BOUNDARY_UNALIGNED`、`TRAFFIC_RANGE_INVALID`、`TRAFFIC_RETENTION_EXCEEDED`。

### 环比与无数据

previous 取紧邻当前窗口之前、小时桶数量相同的区间；尾部不足一小时仍占一个桶。上一窗口超出保留期的部分不会被查询；早于出口统计起点或有异常质量时 complete=false。未请求 compare 时 previous_window/previous/growth_bytes 全为 null。

当前或上一时段有字节或 attempts 的出口/账号都返回；仅上一时段有数据时当前为零，可显示负增长。仅尝试但没有正文的失败请求也可出现；有效的零字节采样可以产生零峰值。完全没有对应小时记录时 peaks=null，history=[]。客户端不要将 null 当作 0 峰值。

## 采集、峰值与 attempts

HTTP 在实际 `Route.observeRoundTrip` 套用 `EgressTransport`，仅持有 route kind、node ID、account ID。外层通用 Transport 和内层出口 Transport 共享同一个观测句柄，请求体、GetBody 重放和响应体不会重复计数。请求体允许在 RoundTrip 返回后继续被读取，归因作用域等请求体关闭和响应完成后才释放。HTTP 101 不按响应正文计数，后续 WebSocket 应用消息通过消息读写入口计数。错误响应已消费的正文同样计入。

WebSocket 池每次 Acquire 通过初始路由校验后创建租约作用域；连接保留同一个可切换句柄。租约释放前切回该连接真实出口/账号的 idle 作用域，平台用户维度回到未归属。预热仅继承全局 Meter，仍可按真实出口/账号归因。非池拨号若 context 带有已解析 Route，则创建并持有自己的作用域至连接关闭。握手与消息不会重复增加同一次租约的 attempts。

attempts 的定义：

1. HTTP：通过 Route.Check 后、进入出口观测 RoundTrip 一次加 1；底层 TCP 自动重试/GetBody 重放只累加实际字节，不额外增加 attempts。业务层重新调用 RoundTrip 或换账号/出口会分别加 1。
2. WS 池：每次 Acquire 通过初始 Route.Check 后加 1，包含随后排队/建连失败；复用已连接 socket 也加 1。Acquire 内部的重试不额外增加。
3. WS 非池：一次通过初始路由校验的 Dial 加 1；池预热的一次拨号流程单独加 1。idle 消息不增加 attempts。
4. 路由前置校验拒绝不计。没有路由元数据的普通上游传输仍保留字节，但不猜测 attempts。全局保护上限降级的尝试在未识别持久化行保留，当前 totals.unidentified 合同只暴露字节。

每次字节 I/O 只向一个原子叶子计数器写入。采样从同一份增量同时派生 API Key、用户、出口、出口账号和整实例，避免分别读取多套计数器造成采样边界差异。scope 创建/释放在请求/租约边界使用锁，字节热路径只读指针和原子加，不查数据库、map 或获取互斥锁。

每秒先合并同一出口下所有账号，再计算 out/in/total 速率；total 先合计同秒 out+in，不能相加各方向或账号各自的历史峰值。延续有效窗口 0.9–1.1 秒；启动不足 1 秒、停止补采样、异常间隔保留字节但不参与峰值。

单次采样不变量（同一新增采样范围）：

- 所有出口合计 + 未识别出口 = service-traffic 上游两方向；客户端字节不参与。
- 同一出口真实账号合计 + unattributed_account = 出口合计。
- 用户/API Key 的一期守恒仍成立；WS idle/预热可以同时属于某出口账号和“未归属平台用户”，这两个维度并不冲突。

## 未识别出口的来源

只对已经纳入服务载荷计数、但没有实际 Route 元数据的流量标为 unidentified：

- `pkg/websearch/brave.go`、`tavily.go` 的 `traffic.Do` 搜索请求。
- `service/image_storage.go` 的远程图片下载。
- `service/batch_image_provider_gemini.go`、`batch_image_provider_vertex.go` 的提供方提交/状态查询/结果读取和清理 HTTP 调用，包括被业务任务显式赋予 Meter 的后台流程。
- `repository/http_upstream.go` 中未经过类型化出口 Route 的兼容传输路径。
- `openai_live.go` 的 `dialLiveSideband` 使用旧代理参数且未提供 Route 的路径，以及其他无 Route 的非池 WS 调用；不能仅凭缺少 Route 猜成直连。
- 作用域结束后的迟到 I/O，或达到下述全局叶子/出口保护上限的流量。

没有 Meter 的巡检、令牌刷新、管理后台请求、数据库/Redis 通信及未接入观测的插件仍不纳入 service_payload_v1，也不会凭空进入 unidentified。**本轮不单独统计后台请求**：需要给全部相关传输接入并定义独立口径，不能为了缩小 WG 差值混入服务总量。

## 存储、容量和质量

新增迁移 `255_service_traffic_egress.sql`，只新增 `service_traffic_egress_buckets` 和 `service_traffic_egress_metadata`，不修改既有迁移/数据。小时表主键为 `(runtime_id,bucket_start,route_type,route_id,account_id)`；account_id=0 为出口合计，-1 为缺失/超限账号残余，正数为真实上游账号。代理和账号不加外键，删除身份不会抹掉流量历史；运行标识沿用 service_traffic_runtimes。

和全实例/用户快照在同一事务保存，最多每批 500 行；runtime sequence 与行 sequence 拒绝重复/旧累计快照。确认保存不清空当前小时累计；旧小时确认后回收。每 5 秒尝试持久化、每小时清理 90 天前数据。元数据保存最早成功持久化的出口样本时间，清理不会重置起点。数据库失败沿用现有约 30–90 分钟未保存积压策略，超出时标 incomplete；崩溃可能丢失最近一次成功保存之后的字节。

活跃出口×账号组合上限 5,000；无字节/新尝试且无作用域引用超过 60 秒后回收。另用当前/待保存小时行数约 10,000 的门槛限制新增账号组合轮换。达到门槛后新的账号组合进入 account_id=-1，仍保留出口总量并设置 overflow。已有活跃组合继续累计，该门槛不是精确内存字节上限。

额外设置 40,000 个用户×出口账号叶子和 1,024 个活跃出口上限，避免两个维度相乘无限扩张；无法分配残余叶子时保留用户/全实例字节，出口降级 unidentified 并标记 overflow。账号缺失也进入 residual，但单纯缺失账号不表示容量溢出。

查询在 PostgreSQL 只读一致性事务中读当前/上一窗口、身份和 WG 小时汇总。身份 SQL 只读 id/name/protocol/deleted 和 id/name/platform/type/deleted，不读取或输出连接地址、端口、凭据、WG 配置/密钥。按精确整数排序，增长量使用有符号大整数；最终结果在 Go 排序，无分页。

同库运行时段重叠时累计字节仍相加，当前窗口所有出口/账号/history 峰值置 null，concurrent_runtimes/incomplete=true。incomplete 还反映无统计起点、窗口早于起点、异常采样、保存积压、过期未关闭运行、overflow；它不是对窗口逐秒连续覆盖的证明。

启用第二期之前的旧 service-traffic 历史不能反推出口；跨启用时点查询不可强制要求旧整实例累计与出口累计相等。守恒适用于同一第二期采样集合。长窗口、高基数查询需要加载各出口账号汇总，尚未进行真实规模压测。

## WG 比较口径

wireguard 字段直接查询 `wireguard_traffic_hourly`，严格使用服务流量同一 `[window.start,window.end)` 桶筛选边界，不使用仅保留 7 天的分钟表。WG 无小时行时三项均为字符串 "0" 且 has_records=false；有零值小时记录则 has_records=true。集成测试用同一窗口与原 WG TrafficReport 逐项比对，并放置窗口外及仅分钟记录验证不串入。

`WG total_bytes - 服务出口 total_bytes` **不能称为精确封装开销**。WG 的 `wg_udp_payload_v1` 是加密 UDP 载荷；服务统计为 HTTP 暴露正文/WS 应用消息，可能包含自动解压后的字节。差值还受后台通信、协议开销、其他载荷、保存时差与计量起点影响，甚至可以为负。前端建议标“隧道与服务载荷差值”，保留符号并提示口径；不要截断负数后称为额外成本。

## 与提示词的差异及原因

1. 新增详情字段 **unattributed_account**，其余既定接口字段保留。超限要求“不计具体账号”与“真实账号之和等于出口”无法同时成立，用显式残余使总量可解释；accounts 只含真实正 ID。
2. routes/accounts 除有字节条目外也包含 attempts-only 和 previous-only 条目，以展示无正文失败和负增长；account_count 仍只看当前非零字节账号。
3. attempts 是上述应用传输/租约尝试数，不等于底层 TCP 连接数、账单请求数或审计 Attempt 记录数。
4. 未选择可选的独立后台统计；只维持现有 service_payload_v1 覆盖边界。已有业务后台任务携带 Meter 的行为保留。
5. 单一原子叶子派生多维增量，而非分别累加多个总计数器；增加叶子/出口保护上限及小时行数门槛，降级情况明确标 overflow。
6. idle/预热仍归属真实出口账号，但不归属已释放的平台用户，也不把 idle 消息当成新 attempts。
7. 独立起点表保留最早样本时间；重复保存/重启不重置。之前的数据不回填，previous 完整性也考虑既有质量标记。
8. WG 小时窗口与约定一致，但差值不承诺是精确“隧道额外占用”，不以零截断掩盖不同口径。
9. 路由身份没有默认猜测：缺少 Route 的旧路径明确 unidentified；不为统计改写现有网络路由/回退策略。

## 验证记录

测试资源：本机容器 `sub2api-traffic-test-db`，独立数据库 `wireguard_test`，每例创建/销毁单独 schema；仓库测试框架额外创建/清理临时 PostgreSQL、Redis 容器。连接信息从本机权限 0600 的临时配置注入，不记录到仓库或输出。

命令工作目录为 backend；GOCACHE 使用 `/private/tmp/sub2api-wg-gocache`。

```sh
make test-unit
GOLANGCI_LINT_CACHE=/private/tmp/sub2api-traffic-lint-cache /private/tmp/sub2api-traffic-tools/golangci-lint run ./...
go test -race -tags=unit ./internal/pkg/traffic ./internal/pkg/egress ./internal/service -run 'Test(Traffic|OpenAIWSConnPool|WebSocketTypedRoute)' -count=1
# 最终采集/WS 生命周期调整后复验：
go test -race ./internal/pkg/traffic ./internal/pkg/egress
go test -race -tags=unit ./internal/service ./internal/handler/admin -run 'Test(Traffic|OpenAIWSConnPool|WebSocketTypedRoute)' -count=1
# SUB2API_WG_TEST_DSN 由私有临时文件注入，未打印。
CI=true go test -tags=integration ./internal/repository -run 'Test(TrafficEgress|TrafficSubject|ServiceTraffic|WireGuard|MigrationsRunner)' -count=1 -v -timeout=240s
go test ./internal/pkg/traffic -run '^$' -bench 'BenchmarkTraffic(EgressCounter|Counter)$' -benchmem -count=3
```

- 新单元/Handler 测试覆盖出口/用户/整实例守恒、同秒与异秒峰值、跨出口账号重试、GetBody 重放、HTTP 失败与先响应后上传、未识别历史保留、容量降级、回收再进入与过期 Ack、并发采样、真实本机 WS 池复用、非池失败正文、预热、对端即刻关闭时的初始化和释放，以及权限/参数/错误 reason。
- 新 PG 测试覆盖重复/旧快照、重启续接、事务回滚、多实例峰值置空、软删/硬删身份、无数据/不存在、环比、90 天清理、WG 同桶窗口及安全字段；同时运行一期与 WG/迁移回归。
- 热路径微基准 Apple M5 Pro / darwin arm64，3 次中位数：用户 scope 上游 Add 1.657 ns/op，出口 Add 1.678 ns/op（约 +1.3%，+0.021 ns），出口 WS switch 2.092 ns/op；全部 0 B/op、0 allocs/op。未计 scope 创建/释放、采样与数据库开销，不代表端到端吞吐或满载性能。
- 全量 `make test-unit` 通过；最终容量尝试计数与 WS 初始化小幅调整后，完整 traffic/egress 包及 service/Handler 相关测试重新以 race 模式通过。
- 最终 golangci-lint 为 0 issues；28 个顶层 PostgreSQL 集成用例通过；热路径基准 3 次均通过；`git diff --cached --check` 无错误。

未执行：前端检查/构建、完整全库集成套件、浏览器联调、真实代理/WG 网络发包、真实付费上游调用、5,000 组合长时间压力测试、正式发布和生产升级。本轮未替换原本地验收容器，既有验收入口运行的仍是此前构建，不能据此确认第二期已上线。

## 本次文件清单

共 25 个文件，仅 backend/ 和 docs/：

```text
backend/internal/handler/admin/traffic_egress_handler.go
backend/internal/handler/admin/traffic_egress_handler_test.go
backend/internal/pkg/egress/attempt.go
backend/internal/pkg/egress/traffic_test.go
backend/internal/pkg/traffic/egress.go
backend/internal/pkg/traffic/egress_test.go
backend/internal/pkg/traffic/http.go
backend/internal/pkg/traffic/metrics.go
backend/internal/pkg/traffic/subjects.go
backend/internal/repository/service_traffic.go
backend/internal/repository/service_traffic_upgrade_integration_test.go
backend/internal/repository/traffic_egress_integration_test.go
backend/internal/repository/traffic_egress_query.go
backend/internal/repository/traffic_egress_save.go
backend/internal/repository/wireguard_store_integration_test.go
backend/internal/server/routes/admin.go
backend/internal/service/openai_ws_client.go
backend/internal/service/openai_ws_pool.go
backend/internal/service/service_traffic.go
backend/internal/service/traffic_egress.go
backend/internal/service/traffic_egress_paths_test.go
backend/internal/service/traffic_egress_test.go
backend/migrations/255_service_traffic_egress.sql
docs/SERVICE_TRAFFIC_ATTRIBUTION_HANDOFF.md
docs/SERVICE_TRAFFIC_EGRESS_HANDOFF.md
```
