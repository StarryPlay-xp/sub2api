# 本机流量套餐监控：后端交接
最后更新：2026-11-02

## 范围与实现位置

仅采集运行 sub2api 的 Linux 本机整机网卡计数；进程内自动启动，不需要独立程序、设备注册、上报接口或多主机列表。不实现服务商接口、自动限速/断网、付费重置或共享流量池。前端由其他任务实现，本次不修改 `frontend/`。

- 核心类型、采集、计算、事务和查询：`backend/internal/hosttraffic/`。
- 路由及鉴权：`backend/internal/handler/admin/host_traffic_handler.go`、`backend/internal/server/routes/admin.go`。
- 生命周期：现有 `ServiceTrafficService` 持有宿主机采集器，应用退出时先停止采集、再关闭数据库。
- 告警：现有 `OpsAlertEvaluatorService.computeRuleMetric` 和告警邮件流程。
- 迁移：`256_host_traffic_quota.sql`，新增五张 `host_traffic_*` 表及 `ops_alert_rules.managed_by`。只新增数据结构，不修改已有业务数据。
- Docker 开启说明：[deploy/HOST_TRAFFIC_CN.md](../deploy/HOST_TRAFFIC_CN.md)。前端链接保持 `https://github.com/StarryPlay-xp/sub2api/blob/main/deploy/HOST_TRAFFIC_CN.md`。

## 最终接口

前缀 `/api/v1/admin/ops/host-traffic`，沿用普通运维接口的管理员鉴权，并拒绝受限/委托管理员。运维监控关闭时为 HTTP 404，`reason=OPS_DISABLED`。数据包裹使用项目现有 `response.Success` 格式；下表描述其中的 `data`。

| 方法与路径 | 请求 | 成功响应 |
| --- | --- | --- |
| GET 根路径 | 无 | `HostTrafficStatus` |
| PUT `/plan` | 套餐字段；首次 `version:null`；可选 `current_used:{value,unit}` | 最新 `HostTrafficStatus` |
| POST `/calibrations` | `{value,unit,mode,plan_version}` | `{kind,status}` |
| GET `/usage?range=24h\|7d\|30d\|90d` | range 缺省 `24h`；兼容公共请求器附带 `timezone` | `{window,state,interfaces,has_records,rx_bytes,tx_bytes,total_bytes,observed_seconds,expected_seconds}` |
| GET `/detail` | 无 | `{days_timezone,days,periods,calibrations}` |

所有字节数为十进制整数字符串；有符号 adjustment 可为负数。浮点字段仅用于比例、偏差和速率。时间为 UTC RFC3339；日历日期是 `days_timezone` 下的 `YYYY-MM-DD`。数组无数据返回 `[]`，可空对象返回 `null`。

### HostTrafficStatus

- `as_of`：本次查询时间。
- `collection`：`state`（`ok/starting/stale/needs_host_mount/unsupported/error`）、`deployment`（`docker/binary/unknown`）、`source`（`host_mount/native/null`）、`started_at/last_sample_at/host_boot_at/error_reason`（可空）。`started_at` 是第一次成功保存的读数，不会因进程重启重置。
- `interfaces[]`：`name/selected/physical/default_route/rx_bytes_per_second/tx_bytes_per_second`。`physical` 可空；`default_route` 为 `{ipv4,ipv6}` 或 null。速率为最近两次同阶段读数的差值除以秒数，读数不可用时为 null。
- `interface_selection`：`mode=auto/manual`、`ambiguous`。
- `since_start`：`{from,rx_bytes,tx_bytes,total_bytes,bound}` 或 null；只累加首次保存读数之后的变化，累计值独立于历史保留，不随清理归零；`bound=exact/lower`。
- `plan`：下述套餐或 null。
- `period`：下述当前账期或 null；没有套餐时为 null。
- `alerts`：`{email_enabled,firing:[{level,kind,since}]}`，`level=warning/critical`，`kind=usage/projection`。列出本功能托管规则的真实 firing 事件；手动规则仍在运维告警页显示。采集与告警评估异步，阈值变化/跨期后的事件会在下次告警评估时恢复。
- `unit_hint`：`{suggested_base:1000|1024,samples:2}` 或 null。

### 套餐与账期

`HostTrafficPlan` 保持约定字段：`version/quota_value/quota_unit/quota_bytes/billing_mode/reset_day/reset_time/timezone/unit_base/interfaces/alerts/updated_at`。

- `quota_value` 使用不带符号、不带指数的十进制字符串，最多 9 位小数；额度只接受 GB/TB，换算后必须大于 0、最多为有符号 64 位整数上限。字节换算向下取整。
- `billing_mode=both/outbound/inbound/max`；VPS 视角 `rx=入站`、`tx=出站`。`max` 是所选网卡本期 rx 合计与 tx 合计的较大值，不是逐分钟 max 的和。
- `reset_day=1..31`；`reset_time=HH:MM` 且分钟为 00/15/30/45；`timezone` 必须是可加载的 IANA 名称，拒绝 `Local`。程序内置 `time/tzdata`。
- 月份不足指定日时取该月末日，下一月仍按原 reset_day 推算。夏令时采用 Go 时区数据库的 `time.Date` 解析规则；涉及跳时/重复时刻的套餐应优先选服务商明确的非跳变时刻或 UTC。
- `interfaces=[]` 自动选择；手动列表最多 16 项，不允许重复或明确排除的虚拟网卡名称。
- `alerts={enabled,warning_percent,critical_percent,projection}`，校验 `0 < warning < critical <= 1000`，允许设置超过 100% 的严重阈值。
- PUT 不接受派生字段 `quota_bytes/updated_at`。version 是后端生成的不透明 UUID；首次 null、后续带上读取的 version。

`HostTrafficPeriod` 保持约定字段：`start/end/observed_from/coverage/bound/rx_bytes/tx_bytes/measured_billed_bytes/adjustment_bytes/adjustment_kind/billed_bytes/quota_bytes/remaining_bytes/overage_bytes/used_ratio/gaps/projection/last_reconcile`。

- `[start,end)`；跨期边界样本的差值归属于结束于该边界的上一账期，不插值。
- `coverage=complete/partial` 只表达期初覆盖，容差两分钟；中途缺口由 bound 与 gaps 表达。
- `gaps={lost_seconds,minor_restarts,unallocated_bytes}`；`projection={billed_bytes,ratio,basis_days}` 或 null；`basis_days` 为实际覆盖秒数/86400，允许小数。
- `last_reconcile={at,provider_bytes,measured_bytes,deviation_ratio}` 或 null；实测为零时偏差为 null。
- `billed=max(0,measured+adjustment)`，`used_ratio=billed/quota`，不是百分数；告警指标会乘 100。等于额度时 remaining 为 `"0"`，超过额度时 remaining 为 null、overage 为差额。

### 校准和错误

基准自动选最近三分钟内**全部选定网卡**的同一分钟读数，且不能早于当前账期。模糊自动选择、选定网卡缺失、只有旧读数，均返回 `HOST_TRAFFIC_NOT_READY`。

- auto + 期初不完整且无现有补齐/覆盖：`kind=fill`，adjustment=max(0,服务商值−实测)。
- 其余 auto：`kind=reconcile`，只记录对账，不改变 adjustment。
- override：以服务商值−实测替换 adjustment，可负数、不可累加。
- 每条记录保存基准时间、服务商数值、同期实测和偏差。`at` 是基准读数时间，不是提交按钮点击时间。
- 最近两条同账期、同统计口径记录的偏差稳定接近 GB 或 TB 的 1000/1024 换算差异时返回 unit_hint；不是固定以 7% 判断，TB 的换算差异更大。
- 套餐、current_used 校准与托管告警更新在同一个数据库事务内；任何失败全部回滚。

| 错误 | HTTP | 说明 |
| --- | --- | --- |
| `HOST_TRAFFIC_PLAN_CONFLICT` | 409 | 创建/更新 version 不匹配，或校准 plan_version 已过期 |
| `HOST_TRAFFIC_PLAN_INVALID` | 400 | metadata.field：quota/billing_mode/reset_day/reset_time/timezone/unit_base/interfaces/alerts/current_used；结构解析错误使用 quota |
| `HOST_TRAFFIC_NO_PLAN` | 409 | 无套餐却请求校准 |
| `HOST_TRAFFIC_NOT_READY` | 409 | 缺少可用的近期读数；携带 current_used 的 PUT 同样整体失败 |
| `HOST_TRAFFIC_CALIBRATION_INVALID` | 400 | 校准金额、单位、mode 或请求结构错误 |
| `OPS_ALERT_RULE_MANAGED` | 409 | 在普通运维规则接口修改/删除托管规则；应改套餐配置 |

JSON 请求体最多 8 KiB，拒绝未知字段和多个 JSON 值。内部读取故障为 500；非法 usage 范围使用已有 `TRAFFIC_*` 错误，`custom` 不受支持。

### usage 与 detail

usage 的窗口对齐与已有服务流量一致：24h 起点向上对齐 UTC 分钟，其他范围向上对齐 UTC 小时，终点是查询时间。`retained_from` 表示小时分段保留边界。observed_seconds 是全部所选网卡已知区间的交集秒数，不能把各网卡秒数相加；expected_seconds 是窗口长度。

有套餐时 days 覆盖当前账期所有日历日，包括尚未来到的日子；无套餐时覆盖 UTC 最近 30 个日历日。首尾日按账期截断。字段保持 `date/start/end/rx_bytes/tx_bytes/billed_bytes/observed_seconds/expected_seconds`，expected_seconds 只计算截至 as_of 的已过去部分，未来日为 0。

periods 最多 12 条，新的在前，包含约定的账期字段、`billing_mode/closed_at`，额外返回完整账期快照字段（如 gaps、projection），前端可忽略。calibrations 最多 20 条，新的在前，字段为 `id/at/kind/provider_bytes/measured_bytes/deviation_ratio/period_start`。

## 相对建议方案的明确补充与差异

1. **不承诺所有重启都只损失 120 秒。** 只有旧读数到新 boot 时间能证明不超过 120 秒才计 minor_restarts；较长间隔计缺口。计数下降、ifindex 变化或已开始采集后新增网卡，也无法知道准确归零/创建时间，因此首个新计数计入，但标记缺口、降为 lower。
2. **同一分钟首次写入固定。** 数据库单例状态行加锁，绝对读数唯一键防重；已保存分钟的重复或迟到批次不再覆盖，避免已计入的差值被重复追加。两个实例不会双计费；此功能仍仅支持同一宿主机共享数据库，不能拿它做多机汇总。
3. **保留小时内的差值分段。** 只保留小时总和无法准确重算 15 分钟边界，因此小时记录附带分钟差值及不可拆分的恢复区间；这些不是原始绝对读数。
4. **无法分配的区间不摊分。** 中断横跨账期、usage 起点或日界线时，无法知道区间内部的分布。整个跨界差值从该窗口实测中排除；账期通过 unallocated_bytes 提示。同一个跨期区间可能在两期都提示，不能把提示值跨期相加当作计费。初次读数追溯至开机且开机在本期前时，不把整段开机以来的量当成中断缺口。
5. **明细日合计可能低于账期合计。** 开机以来的首个读数、同账期内停采后的恢复量可能跨日；账期归属已知但每日分布未知，不按时间比例虚构每日流量。daily billed 只含当天可归属的实测，不摊入 adjustment；`max` 也使逐日 max 的和不等于账期 max。
6. **自动选择保守。** 原生路由只有一个地址族有唯一默认出口时可选该网卡；两个地址族都有出口时必须一致。宿主机挂载模式只在唯一候选且有 device 特征时自动选择，其他情况 ambiguous。Docker 默认路由一律 null。
7. **套餐编辑保留口径隔离。** 保存时先冻结旧套餐已结束账期，再用新套餐重算当前账期。调整额度、告警或显示进制不改变已有字节 adjustment；改变账期边界、计费方向或所选网卡后，旧校准仅保留在历史，不再参与当前口径计算。旧关闭记录永不改写。
8. **预警等级内部映射。** 现有邮件系统将 P0 识别为 critical、P1 为 warning，托管规则使用此映射；本机状态接口仍使用约定的 warning/critical。usage 两级规则使用 `>=`，projection 在达到预测额度 100% 时提醒。
9. **projection 缺省与恢复。** 至少观测三天才预测；基于最近至多七天可归属的实测日均速率。当前 bound=lower 时预测也应作为下限显示。没有预测时，告警指标按零处理以恢复上一账期的预测事件；采集不可用时不产生有效指标，避免误报“恢复”。
10. **额外兼容字段。** 运维规则列表新增可选 managed_by，days basis/expected 语义如上，其他接口字段名及主要错误码均保持原契约。

## 存储、保留与运行

- 原始绝对读数：7 天，每整小时执行清理；每小时每网卡差值分段：13 个日历月，按 UTC 小时截断边界清理。
- 永久保留最近读数游标、首次采集时间和 since_start 累计，不因 7 天清理破坏重启恢复。关闭账期、校准记录不自动删除；接口只取最近 12/20 条展示。
- 关闭发生在跨期后下一次成功采样保存时，长时间停采则恢复后补冻结；已关闭快照使用唯一键 `ON CONFLICT DO NOTHING`，不会被新配置重算。
- 数据库不可用时进程内最多缓冲 180 个分钟批次；超出丢最旧批次，同阶段累计仍由后续绝对计数补回，跨界不可归属部分保持 lower。退出时停止采集器，未保存内存队列不保证持久化。
- 原始计数按 boot_id、网卡名、ifindex、分钟唯一存储；计数下降立即以新读数作后续差值基线，绝不产生负流量；同一网卡只有一个方向下降时，另一个方向只计其增量，防止重复计费。
- 托管规则 managed_by 为 `host_traffic:usage_warning/usage_critical/projection`；这些名称前缀为保留字。三条规则在保存套餐时一起 upsert，手动规则不改动。停用托管规则会恢复其 firing 事件。
- 本机只读访问；接口不返回 boot_id、ifindex、宿主机路径或底层错误文本。当前实现不会尝试联网修复采集失败。

## 验证与未验证边界

本次本地测试覆盖：同阶段恢复/重启/网卡重建/计数下降、整分钟与跨期分配、短月、时区及 15 分钟校验、四种计费、fill/reconcile/override 替换、可信度、三天预测门槛、单位提示、网卡覆盖交集、版本冲突、current_used 失败回滚、重复批次防重、校准记录 ID、告警指标、管理员限制、OPS_DISABLED 和严格 JSON 解析。数据库事务路径使用 sqlmock；它不等同真实 PostgreSQL 集成验收。

部署测试验证 Compose 文件中的只读挂载，以及升级器对模拟 Docker inspect 的匹配、可写/错源/缺失挂载拒绝。Docker inspect 中预期 `/sys` 只有一条顶层 bind 项，递归子挂载不另列；保留升级器原有严格比较，不放宽挂载检查。

本次不连接或操作任何服务器，不发送真实邮件，不 push。仍需用户另行授权：真实 PostgreSQL 迁移/事务及历史恢复、Linux Docker 网卡可读性、容器 boot_id/uptime 与宿主机一致性、实际 updater 升级、宿主机重启连续性、服务商对账、邮件送达和生产规模性能测试。静态系统调用路径检查不能替代 systemd/Docker 实机验收。

已执行的本地检查：

- `go test -race -tags=unit ./internal/hosttraffic ./internal/handler/admin ./internal/service` 的本功能、流量/带宽、既有告警指标筛选测试通过。
- 仓库 golangci-lint 对本次变更检查为 0 issues。
- Wire 依赖装配重新生成，Linux amd64 / CGO_ENABLED=0 后端构建通过，输出仅在临时目录。
- image-updater 15 项测试、主机挂载 2 项离线测试、Compose 安全及运行时资源脚本通过。
- 四份 Compose 与新附加文件的 `docker compose config --quiet` 离线校验通过，未连接 Docker 守护进程。
