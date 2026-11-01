# 服务流量用户 / API Key 归因：后端交接

最后更新：2026-11-01

本次仅修改后端和交接文档。统计沿用 `service_payload_v1` 四方向应用载荷口径，不计费、不限速、不采集请求内容、IP 或 Key 明文/哈希。前端由另一任务在同一仓库接入。本次不推送、不发布、不修改版本或线上服务。

## 接口与前端接入

三个接口均挂在现有管理后台认证路由下，仅完整管理员可用：受限管理员 403，运维监控关闭 404，存储/采样器未初始化 503，读取失败 500。外层仍是 `{code,message,data}`。

| 接口 | 参数 |
| --- | --- |
| `GET /api/v1/admin/ops/traffic-ranking` | 必填 `subject=user|api_key`；`range/start/end`、`interval=hour`、`sort`、`order=asc|desc`、`page`、`page_size`、`q`、`user_id`、`compare=previous` |
| `GET /api/v1/admin/ops/traffic-subjects/:type/:id` | `type=user|api_key`，正整数 id；`range/start/end`、`interval=hour`、`compare=previous` |
| `GET /api/v1/admin/ops/traffic-live` | 必填 `subject`；`limit` 默认 10、最大 50；`sort=avg_60s|rate`，默认 avg_60s |

默认时间范围 24h，区间查询默认且只允许 hour。range 支持 24h/7d/30d/90d/custom，窗口解析复用 `ResolveTrafficWindow`；历史 custom 边界须对齐 UTC 整点，省略 end 或合法未来 end 截断至当前时刻。未来 end 最多 25 小时。timezone 接受并忽略。未知参数、重复参数返回 400，空的分页参数也视为非法。

排行默认 `sort=total_bytes&order=desc&page=1&page_size=20`，page_size 最大 200。排序支持提示词列出的全部十种字段。null 始终排最后，同值按主体 id 升序。`sort=growth` 必须带 `compare=previous`。

`share`、`totals` 和集中度比例的分母始终为当前窗口全站已归属加未归属字节。`user_id` 仅支持 api_key，影响候选主体、total 和 concentration；q 不区分大小写，筛选 items，并使 total 返回筛选后的可分页结果数。concentration（包括 subjects）、totals 和 share 的全站分母不受 q 影响。例如 45 个候选 Key 中搜索命中 2 个，total=2，concentration.subjects=45。

环比选紧邻当前窗口之前、小时桶数相同的区间：当前尾部不足一小时仍计一个桶。上一窗口早于归因启动、超出保留期或存在异常样本/进程状态等质量问题时 complete=false；过期部分不会因为清理任务尚未执行而混入结果。只在上一窗口有字节或请求的主体也返回，当前为零。历史不补空桶。

字节使用十进制整数字符串，growth_bytes 可以带负号；bytes_per_request 为字节总数除以请求数后向下取整的字符串，requests=0 时 null。比率/速率为浮点数 B/s。`active_seconds` 是有字节的有效样本次数，不是精确墙钟持续时间。`active_avg_bytes_per_second` 包含所有累计字节除以有效活跃样本次数，遇到异常采样请结合 incomplete 展示。

### 响应类型（与提示词一致）

```ts
type Direction = 'client_in' | 'client_out' | 'upstream_out' | 'upstream_in'
type Bytes = Record<Direction, string>
type Peak = { bytes_per_second: number; at: string; window_seconds: number }
type SubjectPeaks = Partial<Record<'in' | 'out' | 'total', Peak>> | null
type SubjectType = 'user' | 'api_key'
type RankingSort =
  | 'total_bytes' | 'in_bytes' | 'out_bytes' | 'requests' | 'bytes_per_request'
  | 'peak_total' | 'peak_in' | 'peak_out' | 'active_avg_rate' | 'growth'

type SubjectIdentity = {
  subject_type: SubjectType
  user_id: number
  user_email: string
  username: string
  user_deleted: boolean
  api_key_id: number | null        // subject_type=user 时为 null
  api_key_name: string | null
  api_key_deleted: boolean | null
  group_id: number | null          // Key 当前所属分组；user 行为 null
  group_name: string | null
}

type SubjectStats = SubjectIdentity & {
  bytes: Bytes
  in_bytes: string
  out_bytes: string
  total_bytes: string
  share: number | null             // total_bytes / totals.all_total_bytes，0–1；分母为 0 时 null
  requests: number
  bytes_per_request: string | null // requests=0 时 null
  active_seconds: number
  active_avg_bytes_per_second: number | null // total / active_seconds；active_seconds=0 时 null
  peaks: SubjectPeaks
  active_api_keys: number | null   // 仅 user 行：窗口内有流量的 Key 数
  previous: { total_bytes: string; peaks: SubjectPeaks } | null // 仅 compare=previous
  growth_bytes: string | null      // 有符号十进制字符串，当前减上一时段；仅 compare=previous
}

// TrafficWindow 即现有 service-traffic 响应 report 中的窗口字段：
// range / interval / start / end / requested_start / requested_end / retained_from / as_of / end_clamped
type RankingWindow = TrafficWindow & {
  metric_basis: 'service_payload_v1'
  attribution_started_at: string | null // 最早的用户/Key 维度样本时间，早于此没有分维度数据
  has_records: boolean
  incomplete: boolean
  concurrent_runtimes: boolean
  subject_overflow: boolean
}
type PreviousWindow = { start: string; end: string; complete: boolean } | null

// 1) GET /api/v1/admin/ops/traffic-ranking
// 参数：subject（必填 user|api_key）、range、start、end、interval（仅 hour）、
//      sort（默认 total_bytes）、order（desc|asc，默认 desc）、page（默认 1）、
//      page_size（默认 20，最大 200）、q、user_id（仅 subject=api_key）、compare（仅 previous）
type TrafficRankingData = {
  window: RankingWindow
  previous_window: PreviousWindow
  subject: SubjectType
  sort: RankingSort
  order: 'desc' | 'asc'
  page: number
  page_size: number
  total: number                    // q 与 user_id 过滤后的可分页结果数
  items: SubjectStats[]
  totals: {
    attributed: { bytes: Bytes; total_bytes: string; requests: number }
    unattributed: { bytes: Bytes; total_bytes: string }
    all_total_bytes: string
  }
  concentration: { subjects: number; top1: number | null; top5: number | null; top10: number | null } // 按 total_bytes 计算的占比
}

// 2) GET /api/v1/admin/ops/traffic-subjects/:type/:id     type = user | api_key
// 参数：range、start、end、interval（仅 hour）、compare（仅 previous）
type SubjectBucket = {
  start: string
  interval: 'hour'
  bytes: Bytes
  requests: number
  active_seconds: number
  peaks: SubjectPeaks
}
type TrafficSubjectDetailData = {
  window: RankingWindow
  previous_window: PreviousWindow
  summary: SubjectStats        // 窗口内无数据时字节为 "0"、peaks=null
  history: SubjectBucket[]     // 按 start 升序，不补空桶
}

// 3) GET /api/v1/admin/ops/traffic-live
// 参数：subject（必填）、limit（默认 10，最大 50）、sort（avg_60s|rate，默认 avg_60s，均按 total 排）
type RateTriple = { in: number; out: number; total: number } // B/s
type TrafficLiveData = {
  subject: SubjectType
  state: 'warming_up' | 'sampled' | 'invalid_interval'
  sampled_at: string
  window_seconds: number
  host: { rate: RateTriple; avg_60s: RateTriple }
  unattributed: { rate: RateTriple; avg_60s: RateTriple }
  items: Array<SubjectIdentity & {
    rate: RateTriple            // 最新 1 秒采样
    avg_60s: RateTriple         // 最近 60 秒平均
    avg_60s_seconds: number     // 60 秒平均实际覆盖的秒数（进程刚启动时会更短）
    in_flight: number           // 进行中的请求或连接数
    last_active_at: string | null
  }>
}
```

### 错误 reason

- `TRAFFIC_SUBJECT_INVALID`：主体类型非法或详情 id 非正整数。
- `TRAFFIC_SORT_INVALID`：排序字段或 order 非法。
- `TRAFFIC_COMPARE_REQUIRED`：growth 缺少 compare=previous。
- `TRAFFIC_PAGE_INVALID`：page、page_size 或实时 limit 非法。
- `TRAFFIC_FILTER_INVALID`：未知/重复参数、非法 user_id 组合、非法 compare。
- `TRAFFIC_SUBJECT_NOT_FOUND`：详情主体既不存在于身份表，也没有保留中的历史记录，HTTP 404。
- 继续使用窗口错误：`TRAFFIC_INTERVAL_INVALID`、`TRAFFIC_DATE_INVALID`、`TRAFFIC_BOUNDARY_UNALIGNED`、`TRAFFIC_RANGE_INVALID`、`TRAFFIC_RETENTION_EXCEEDED`。

## 采集与归属规则

网关中间件为每个请求创建独立句柄。普通及 Google API Key 鉴权的普通模式、简易模式成功分支均绑定 `(user_id,api_key_id)`，请求数只增加一次，网关处理退出时减少 in_flight。绑定发生在鉴权中间件决定放行处；凭据、IP/状态、订阅或余额等前置检查未通过时不完成绑定。鉴权前已读取/写出的字节保留为未归属，不回迁。

Read/Write 热路径仅检查指针和执行原子计数，不查主体 map、不获取互斥锁、不访问数据库。采样器每秒读取同一组 Key 与未归属增量，由这些增量推导用户及整实例总数，从结构上避免“同时分别读整实例计数器和主体计数器”造成的采样边界差异。用户峰值先合并同一次采样的名下 Key，再求速率；并非相加 Key 的历史峰值。

0.9–1.1 秒以外、启动不足一秒和停止补采样只保留字节，不参与峰值。有效但零字节采样可以有零速率；没有有效样本的峰值未知。

WS 上游连接持有可切换的计数句柄，每次 Acquire 返回租约时切到该请求，Release 在连接重新可用之前切回未归属；池预热只继承全局 Meter。上游握手失败正文仍跟随发起请求。实时语音后台 observer 用记录中的 user_id/api_key_id 创建背景作用域，连接结束即释放。批量图片 provider processor 从已读取的任务记录取得主体，包住任务的提供方查询、结果下载及清理流程；APIKeyID 缺失则未归属。后台作用域不增加 requests 或请求 in_flight。

### 传输调用点核查

- 网关路由注册复用 ServiceTraffic：`/v1`、`/v1beta`、根路径别名、`/backend-api/codex`、`/antigravity/*`；普通和 Google 鉴权都写入原请求 context 中的句柄。
- `repository/http_upstream.go`、`pkg/egress/attempt.go`：HTTP Transport 从请求 context 取句柄，重试、路由包装继续继承；嵌套包装沿用既有防重复计数。
- `pkg/websearch/{brave,tavily}.go`、`service/image_storage.go`、批量图片 Gemini/Vertex 提供方：`traffic.Do` 保留调用方 context。
- `openai_ws_client.go`：读写都使用拨号时保存的句柄；池连接传入的是可切换句柄，非池连接保留当前请求/observer 作用域。
- `openai_ws_forwarder_ingress.go`、`openai_ws_v2_passthrough_adapter.go`、`openai_ws_client_read.go`：虽然部分读取使用 Background 作为取消上下文，计数句柄仍显式来自原始 controlCtx。
- `grok_audio.go`、`openai_live.go` 客户端侧读写：显式使用请求 context 的句柄。

### 未归属来源

1. 鉴权未放行或还没完成绑定的网关字节，含失败响应。
2. WS 池空闲、预热期间的应用消息或失败握手正文；控制帧仍不计入应用载荷。
3. 批量图片任务缺少用户或 Key id，或 observer 记录缺少任一 id。
4. 主体数/小时积压达到上限后新主体的字节；同时标记 subject_overflow。
5. 仅携带全局 Meter 的已纳入观测后台传输，或作用域关闭后的迟到写入。

未经过已观测传输组件的独立插件、管理后台流量及数据库/Redis 通信仍不计入整个 service_payload_v1，不应称为“未归属服务流量”。

## 存储、内存与数据质量

迁移 `254_service_traffic_subjects.sql` 仅新增普通列小时表 `service_traffic_subject_buckets` 和单行 `service_traffic_attribution_metadata`。后者保存最早成功持久化的归因样本时间，避免清理 90 天历史后误将归因起点向后移动。无主体外键，以保留软删除/硬删除后的流量历史。运行标识沿用原 service_traffic_runtimes。

主体小时桶与原整实例快照在同一事务保存，每批最多 500 行；runtime sequence 拒绝重复/旧快照，主体行也按 sequence 替换累计值。保存确认不会清空当前小时累计，未变化的主体桶不重复写入。旧小时确认后移除。失败保留待保存桶，最长积压沿用原约 30–90 分钟边界，超出明确设置 incomplete；异常退出仍可能丢失最近成功保存后的字节。清理任务每小时清理 90 天前主体桶。

活跃 Key 计数器最多 20,000 个；最近 60 秒内无字节、无新请求且没有作用域引用时回收。用户计数由这些 Key 合并，最多派生同数量用户。当前及未保存小时累计桶另外限制在约 40,000 个主体行的量级，防止一个小时轮换大量 Key 绕过活跃上限；到达边界时新主体走未归属。已经存在的活跃主体继续计数，因此边界不是逐字节硬内存限额。已回收计数器的当前小时累计保留到换小时并保存确认，重新活跃不会覆盖历史累计。

实时每个主体保留约一分钟采样，包含空闲零流量秒。为兼容最短 0.9 秒正常采样，采样保留上限为 68 个记录，并按时间裁剪到 60 秒。avg_60s 的分母为当前进程实际已观察时长与 60 秒的较小值；进程已运行很久但刚出现的新主体也除以 60，前面的零流量时间有效。跨 60 秒边界的最旧延迟样本按覆盖比例估算，异常调度时仍显示 invalid_interval，不将该速率解释为精确一秒。

实时排行仅读内存中的流量；入榜主体名称按需批量读身份表并缓存 60 秒，不查流量表。实时结果和 in_flight 是最新采样状态，受 1 秒刷新间隔影响。历史查询使用只读一致性事务，数据库先聚合小时行，再在服务层对每主体汇总做排序、筛选和分页。整数比较不转换为浮点数，避免超过 JS 安全整数时相邻字节数错序。

同库运行时间重叠时，该查询窗口内 summary/history peaks 一律 null，concurrent_runtimes/incomplete=true；累计字节和请求数仍合并。实时仍只表示当前响应进程。软删除身份不加 deleted_at 排除条件，仍返回原邮箱/用户名/Key 名和删除标记；读取列明确不包含 Key 明文或哈希。硬删除只能返回保留的 id 和删除标记，名称无法恢复。

## 与提示词的差异和补充

1. **三个接口的路径、字段、排序项和参数不变；按后续需求纠正 total 语义为 q 过滤后的命中数**，用于分页和 CSV 导出结束判断。原提示词将 total 与集中度范围写在一起；现在只有集中度（含 subjects）不受 q 影响，totals/share 继续保持全站口径。额外拒绝重复参数以避免歧义；非法实时 limit 使用 TRAFFIC_PAGE_INVALID。
2. 选择“绑定前字节保留未归属”，不回迁；计数的放行边界包含鉴权中间件现有的前置检查。
3. 整实例计数改为由同一次采样的主体增量推导，避免维护两套原子计数导致采样瞬间不守恒；原整实例 API 及数据口径不变。
4. 迁移额外新增起点元数据表，保证 90 天清理不改变 attribution_started_at。已有迁移未改写。
5. 聚合在 SQL、最终排序/分页在 Go，以共享排行/详情的精确整数和环比逻辑；高基数长窗口仍需加载各主体汇总，不是只读取一页的数据库分页。尚无真实 20,000 主体长窗口压测结论。
6. 一分钟缓存允许最多 68 个记录而非固定 60 个，避免 0.9 秒采样抖动让一分钟平均漏掉最早几秒；仍按 60 秒真实时间裁剪。
7. 在活跃上限之外增加小时累计积压约束；未活跃计数器回收后，当前小时累计留到保存并换小时，以保证再次活跃不重置已存累计。
8. bytes_per_request 向下取整；排序使用除法前的精确比值。previous_window.complete 还会反映已有数据质量异常，而不仅检查起点和保留期。
9. 启用前的旧整实例历史不能倒推出用户或 Key。三个新接口 totals 只涵盖新增归因采样，不能与跨启用时点的旧 service-traffic 总量强行要求相等；同一新采样范围内满足守恒。

## 验证与交付状态

测试连接信息仅来自本机已有独立 `wireguard_test` 容器，测试再创建独立 schema；现有仓库测试框架另会创建并清理临时 PostgreSQL/Redis 容器。不使用生产数据库，临时凭据不写入仓库。

实际执行（工作目录均为 backend；缓存指向已有临时可写目录）：

```sh
GOCACHE=/private/tmp/sub2api-wg-gocache make test-unit
GOCACHE=/private/tmp/sub2api-wg-gocache GOLANGCI_LINT_CACHE=/private/tmp/sub2api-traffic-lint-cache /private/tmp/sub2api-traffic-tools/golangci-lint run --timeout=30m
GOCACHE=/private/tmp/sub2api-wg-gocache go test -race -tags=unit ./internal/pkg/traffic ./internal/server/middleware ./internal/handler/admin ./internal/service -run 'Test(Traffic|ServiceTraffic|OpenAIWSConnPool|BatchImageProviderTraffic)' -count=1
# DSN 由权限为 0600 的本机临时配置注入，下面仅为去敏命令表示。
SUB2API_WG_TEST_DSN='<本机隔离测试库>' GOCACHE=/private/tmp/sub2api-wg-gocache CI=true go test -tags=integration ./internal/repository -run 'Test(TrafficSubject|ServiceTraffic|WireGuard|MigrationsRunner)' -count=1 -v -timeout=240s
GOCACHE=/private/tmp/sub2api-wg-gocache go test ./internal/pkg/traffic -run '^$' -bench BenchmarkTrafficCounter -benchmem -count=3
```

- 全量单元测试通过；保存确认优化后直接相关的单元测试以 race 模式重新验证。
- golangci-lint：0 issues。系统 PATH 原无该命令，使用已有本机测试工具目录内的版本。
- 上述 traffic/中间件/Handler/WS 池/批量任务/实时缓存相关 race 检查通过。
- 集成回归 26 个顶层用例通过，包含重复和旧快照、事务失败回滚、重启、并发实例峰值失效、软删除、q/user_id、同值分页、上一时段独有主体、保留期裁剪、90 天清理和旧版本升级。
- Handler 覆盖 403/404/503/500、参数 reason、默认小时粒度和空数组响应；真实普通/Google 鉴权及标准/简易模式四个分支有归因测试。
- 热路径基准在 Apple M5 Pro / darwin arm64 执行 3 次：原 Add 中位数 1.557 ns/op；新未归属 1.619 ns/op（约 +4.0%）；已归属 1.599 ns/op（约 +2.7%，增加 0.042 ns）；WS 切换句柄 2.032 ns/op（约 +30.5%，增加 0.475 ns）。均为 0 B/op、0 allocs/op。这是无争用微基准，不代表端到端吞吐或 20,000 主体压测。

初次交付 `bf0ed0768` 未执行前端测试/构建或替换验收应用。以下补充记录本日后续已授权的搜索修正与本地整体构建。真实付费供应商调用、20,000 主体长时间负载测试、正式发布和生产升级均未执行。

## 搜索总数修正与本地验收环境（2026-11-01）

排行 `total` 在 q 过滤后、分页前计算；`concentration.subjects/top1/top5/top10`、`totals` 和 `share` 分母保持过滤前范围。单元测试覆盖 45 个 Key 搜索命中 2 个、第二页、超出末页、零命中，并核对集中度、全站总量和匹配项 share 不变。PostgreSQL 集成断言同步改为搜索命中数。

本轮检查：

- `go test -tags=unit ./internal/service -run '^TestTrafficRanking' -count=1`：通过。
- 本机隔离库 `wireguard_test` 上 `go test -tags=integration ./internal/repository -run '^TestTrafficSubjectPersistenceQueriesAndRetention$' -count=1 -v -timeout=240s`：通过；使用临时 schema。
- `golangci-lint run --timeout=30m`：0 issues（使用已有临时工具及缓存目录）。
- 前端依次执行 `./node_modules/.bin/vitest run src/i18n/__tests__/localeKeyCompleteness.spec.ts`、`./node_modules/.bin/vue-tsc -b`、`./node_modules/.bin/vite build`：均通过，i18n 3 项通过。系统 pnpm 首次尝试因自动安装依赖失败而退出，未通过重装解决；后续直接使用工作目录已有工具完成相同构建流程。
- `./node_modules/.bin/vitest run src/views/admin/traffic/__tests__ src/components/admin/traffic src/api/admin/__tests__/trafficSubjects.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts`：7 个文件、58 项通过。
- 从当前工作目录（包括未提交前端）执行 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags embed -trimpath -ldflags='-s -w -X main.BuildType=source'`：通过，输出到本机验收临时目录。未变更版本号。

验收入口：`http://127.0.0.1:18085/admin/traffic`；沿用已有测试登录账号，凭据仍在本机私有临时配置中，不写入仓库或日志。

| 用途 | 名称 |
| --- | --- |
| 应用容器 | `sub2api-traffic-acceptance-app-1` |
| 数据库容器 | `sub2api-traffic-acceptance-postgres-1` |
| 数据库 | `traffic_acceptance` |
| Redis 容器 | `sub2api-traffic-acceptance-redis-1` |
| 新增本机合成模型容器 | `sub2api-traffic-acceptance-fixture-1` |

复用验收数据库和数据卷，只替换应用程序并重建应用容器，旧测试程序已备份。应用启动自动执行迁移 254，已从迁移表及主体小时表确认。原先的 WG 合成账号在本轮操作前已被软删除，因此未复用旧流量生成脚本，也未请求环境中其他模型账号；改为专用测试分组、两个测试 Key 和仅验收容器网络内可见的合成模型容器，不开放额外宿主机端口。

产物核对：本次生成 `TrafficView-BA9fitt3.js`；构建 JS 中存在 `/admin/traffic`、“流量监控”和三个归因接口。实际访问 `/admin/traffic` 返回 200，并对页面引用及相关的 8 个 JS 资源逐一核对 SHA-256，全部与当前工作目录构建产物一致。构建前后按文件内容哈希及 Git index 校验，前端源码、未提交内容和暂存状态均未变化。

接口联调：两个 Key 各执行一次流式、一次普通请求，4 次均返回 200；流式响应收到 `[DONE]`，执行期间实时接口返回非零速率和 in_flight。保存后排行有 2 个 Key，搜索“搜索对照”仅命中 1 个且 total=1、concentration.subjects=2，空搜索结果 total=0；全站 totals、用户/Key 合计与详情历史校验通过。首轮生成已归属载荷 469304 字节。

本轮未重跑后端全量单元测试、全部前端测试、前端 lint 或 race；执行了上述直接相关回归、静态检查、完整构建和实际 HTTP 联调。未进行浏览器交互式人工验收，该步骤留给用户；实时列表只展示最近一分钟活跃主体，停流后消失是正常行为。

## 本次修改文件清单

- `backend/internal/handler/admin/traffic_subject_handler.go`
- `backend/internal/handler/admin/traffic_subject_handler_test.go`
- `backend/internal/pkg/traffic/metrics.go`
- `backend/internal/pkg/traffic/subjects.go`
- `backend/internal/pkg/traffic/subjects_test.go`
- `backend/internal/repository/service_traffic.go`
- `backend/internal/repository/service_traffic_upgrade_integration_test.go`
- `backend/internal/repository/wireguard_store_integration_test.go`
- `backend/internal/repository/traffic_subject_integration_test.go`
- `backend/internal/repository/traffic_subject_query.go`
- `backend/internal/repository/traffic_subject_save.go`
- `backend/internal/server/middleware/api_key_auth.go`
- `backend/internal/server/middleware/api_key_auth_google.go`
- `backend/internal/server/middleware/service_traffic.go`
- `backend/internal/server/middleware/service_traffic_test.go`
- `backend/internal/server/middleware/service_traffic_auth_test.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/service/batch_image_processor.go`
- `backend/internal/service/openai_live.go`
- `backend/internal/service/openai_ws_pool.go`
- `backend/internal/service/service_traffic.go`
- `backend/internal/service/traffic_subject_paths_test.go`
- `backend/internal/service/traffic_subjects.go`
- `backend/internal/service/traffic_subjects_test.go`
- `backend/migrations/254_service_traffic_subjects.sql`
- `docs/SERVICE_TRAFFIC_BACKEND_HANDOFF.md`
- `docs/SERVICE_TRAFFIC_ATTRIBUTION_HANDOFF.md`
