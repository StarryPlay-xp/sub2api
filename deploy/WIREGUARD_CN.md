# 应用内 WireGuard 出口
> 最后更新日期：2026-10-31

用于 Ubuntu 22.04、Ubuntu 24.04、Debian 13 部署。先使用包含本功能的构建；现有远端 `latest` 镜像不代表已经包含本次代码。

## 工作方式

在「IP管理 → 添加 WireGuard」粘贴客户端配置，再在账号的代理选择中绑定节点，包括 API Key 账号。多个账号共享同一节点的隧道和累计流量，也可配置多个节点。配置只支持一个 Peer，必须提供隧道地址、DNS、Endpoint 和 AllowedIPs；不执行 PreUp/PostUp 等脚本。目标域名的 DNS 查询经过隧道；仅 Endpoint 自身的域名在建立隧道前由宿主机解析。未覆盖的地址族不会回退直连。

应用通过 Go 用户态网络栈建立隧道，无需客户端宿主机安装 WireGuard、修改默认路由、提供 `/dev/net/tun`、授予 `NET_ADMIN` 或另建 SOCKS 服务。原有数据库、Redis 和其他容器继续使用原网络。客户端须能向配置的 Endpoint 发送 UDP；出口服务器须已配置对应 Peer、转发和公网出口，安全组放行的是 UDP 端口。

节点不可用、暂停、失效或账户绑定无法确认时，请求停止；已选定 WG 的同一次请求不切换其他账号或出口。确认隧道故障后需要手动恢复，恢复会创建新配置代次。暂停或编辑会关闭相关连接，包括正在进行的流式请求。主动解除账号绑定属于明确修改出口，之后的新请求才可使用新出口。

不支持把 WG 账号与应用外的 OpenAI OAuth 传输插件或自定义 Anthropic OAuth 中转同时使用。数据库和请求入口均有拦截。

## 客户端配置模板

参见 [wireguard-client.conf.example](wireguard-client.conf.example)。该文件仅是模板，未包含可用密钥。替换客户端私钥、服务器公钥、Endpoint、服务器分配的客户端隧道地址；DNS 也必须能经该隧道访问。示例的 `1.1.1.1` 是公共 DNS，并非要求使用服务器自身作为 DNS。

此模板只允许 IPv4。需要 IPv6 时，服务器必须实际提供 IPv6 转发和出口，并为 `Address` 增加分配给客户端的 IPv6 地址、为 `AllowedIPs` 增加 `::/0`。仅填写 `::/0` 不会自动获得 IPv6 出口。若服务器要求预共享密钥，再填写 `PresharedKey`；否则保持该行注释。只支持一个 Peer，不要粘贴服务端配置或添加 `PostUp`、`PostDown`、`Table`、`SaveConfig` 等字段。`ListenPort` 可省略，填写时仅支持 `0`。

模板中的 WG 私钥/公钥是 WireGuard 格式的 Base64 密钥，不能用 `WIREGUARD_KEY_FILE` 的十六进制加密密钥替代。

## 一次性配置加密密钥

`WIREGUARD_KEY_FILE`（或 YAML 的 `wireguard.key_file`）指向一个长期保存的密钥文件，内容为 64 个十六进制字符。该密钥用于加密数据库中的 WG 配置，与 Peer 私钥是两种不同密钥。文件权限必须为仅所有者可读写（如 `0600` 或 `0400`），且应用运行用户能读取。

必须分别备份数据库和该文件。升级、重建容器、重启时沿用原文件。**不要在每次启动时重新生成密钥。** 当前没有在线主密钥轮换功能；丢失文件后必须重新录入各节点的原始配置。未指定文件时普通代理仍可运行，WG 创建和连接被阻止；指定文件但不可读、格式或权限错误时应用启动失败。

### systemd 二进制部署

使用仓库默认的 `sub2api` 服务用户时，在服务器执行一次：

```sh
sudo install -d -o sub2api -g sub2api -m 700 /opt/sub2api/secrets
sudo -u sub2api sh -c 'umask 077; set -C; openssl rand -hex 32 > /opt/sub2api/secrets/wireguard.key'
```

`set -C` 防止覆盖已经存在的密钥。在 `systemctl edit sub2api` 的覆盖配置中添加：

```ini
[Service]
Environment=WIREGUARD_KEY_FILE=/opt/sub2api/secrets/wireguard.key
```

之后执行 `sudo systemctl daemon-reload` 和 `sudo systemctl restart sub2api`。重启会中断当前请求，应在维护窗口操作。如果自定义了服务用户，请相应调整目录和文件所有者。

### Docker Compose 部署

在 Compose 所在目录创建独立密钥文件，仓库 Dockerfile 的应用用户 UID/GID 为 `1000:1000`：

```sh
sudo install -d -m 700 secrets
sudo sh -c 'umask 077; set -C; openssl rand -hex 32 > secrets/wireguard.key'
sudo chown 1000:1000 secrets/wireguard.key
```

创建本地 `compose.wireguard.yml`，合并到现有部署（密钥文件不要加入 Git）：

```yaml
services:
  sub2api:
    environment:
      WIREGUARD_KEY_FILE: /run/sub2api-secrets/wireguard.key
    volumes:
      - ./secrets/wireguard.key:/run/sub2api-secrets/wireguard.key:ro
```

```sh
docker compose -f docker-compose.yml -f compose.wireguard.yml up -d sub2api
```

若自行改变容器 UID/GID，应使文件所有者与之相符。不要放宽为所有用户可读，也不要通过环境变量传递 Peer 私钥。无需映射客户端 WG 监听端口；它使用出站 UDP。只读根文件系统和删除全部 Linux capabilities 的 WG 核心测试已通过，但完整应用仍需原有的数据、日志等可写目录。

## 操作与诊断

1. 页面确认部署加密密钥可用后创建节点。编辑时配置框留空会保留原配置，私钥不会回显或导出。
2. 运行节点的 IPv4/IPv6 出口验证，只测试配置支持的地址族。探测通过固定的 HTTPS 回显服务，不携带账号令牌。
3. 给测试账号选择节点，再运行账号测试或实际请求。暂停节点后该账号的网络请求应失败，恢复后重新验证。
4. 安全组或网络修复后使用「手动恢复」；不要通过清空代理绕过故障。异常退出后的旧运行租约过期时，节点会暂停并标记统计可能不完整。

同一节点在数据库中只允许一个应用进程持有有效运行租约。多副本部署不会自动建立多个相同 Peer；非持有者会拒绝该节点请求。需要多副本高可用时必须另行设计请求分配，不能将本功能当作自动迁移的分布式 VPN。

## 流量与出口证据的含义

- 流量统计为应用 UDP socket 实际收发的**加密 UDP 载荷**，包括握手、保活、DNS、探测和重传；不含外层 IP/UDP 报头、Endpoint 初始域名解析及其他进程流量，不能直接当作云厂商账单。
- 每秒采样，速度为最近约 1 秒平均（按实际采样间隔计算）；页面每 2 秒刷新，累计值约每 5 秒持久化。历史接口保留发送、接收和同时刻合计的 1 秒平均峰值及发生时间；旧历史没有峰值时返回未知，异常采样不参与峰值。分钟明细保留 7 天（页面展示最近 24 小时），小时明细保留 90 天。运行期高水位保留以防重复计数。服务流量与 WG 峰值接口见 [后端交接说明](../docs/SERVICE_TRAFFIC_BACKEND_HANDOFF.md)。异常退出、长时间无法写库可能丢失尾部统计，页面会标记不完整。
- 活跃的 WG/直连出口独立探测，约 5 分钟刷新，证据 10 分钟过期。请求记录按实际连接地址族、配置代次及是否复用连接保存当时快照；后续探测不改写历史。
- HTTPS 回显结果只证明对应路径的观测出口，不能保证每个上游看到相同 IP；不是上游回执。原客户端 IP 字段不变。未观测、已过期、缺失记录均明确显示，不能推断为直连或成功。
- 请求证据在响应结束或流关闭后异步写入，WebSocket 获取/复用连接时单独记录。写入队列有上限；写库失败或队列满时计入丢弃计数，页面会提示。出口证据保留 90 天，与流量明细分开清理。

## 验证与实现入口

`backend/scripts/test-wireguard-containers.sh` 默认编译当前主机架构的 Linux 测试程序（可设置 `SUB2API_WG_TEST_ARCH=amd64` 或 `arm64`），依次运行 Ubuntu 22.04/24.04、Debian 13 容器，使用非 root、只读根文件系统、独立的内存临时目录和 `--cap-drop ALL`。默认只运行隔离测试；远端验证需显式提供以下**文件路径**：

```sh
export SUB2API_WG_TEST_CONFIG=/absolute/path/client.conf
export SUB2API_WG_TEST_CA_FILE=/absolute/path/ca-certificates.crt
export SUB2API_WG_TEST_EXPECTED_EXIT=/absolute/path/expected-exit.txt
backend/scripts/test-wireguard-containers.sh
```

客户端配置必须仅所有者可读，预期出口文件只存测试服务器的出口 IP。不要把原始配置或私钥打印到测试日志。相同 Peer 密钥不可同时在多个测试实例使用，避免服务器将 Peer 的远端地址切换到另一个实例。

实现入口：`backend/internal/pkg/wgtunnel`（隧道、加密、统计），`backend/internal/pkg/egress`（路径、连接及证据），`backend/migrations/250_wireguard_egress.sql`（持久化与保护），`frontend/src/components/admin/proxy/WireGuardDialog.vue`（节点操作）。三种发行版的容器测试共享 Docker Linux 内核，不代表已在三种独立宿主机内核上验收；正式部署还需验证当地网络和真实供应商账号。


### 本次验证边界

- Ubuntu 22.04、24.04、Debian 13 的 **arm64 容器**通过完整 WG 测试和真实服务器出口验证。
- 真实 **x86_64 Linux 测试服务器**（Ubuntu 26.04）以 `nobody` 用户通过本机双 Peer、HTTP/2、TLS 指纹、SSE 和 WebSocket 测试；Linux amd64 内嵌前端的应用构建通过。
- 本机 Docker 的 amd64 跨架构模拟在默认 CPU 加速路径出现 TLS 超时；仅为测试设置 `SUB2API_WG_TEST_GODEBUG=cpu.all=off` 后，三个发行版的容器与公网出口测试全部通过。同一程序在真实 x86_64 上不需要此设置。该选项不是生产部署要求，仍不能宣称三种原生 x86_64 宿主机均已验收。
- 已在独立应用数据库中验证节点创建、API Key 账号绑定、真实账户请求出口快照、暂停和手动恢复、探测超时锁定、密钥/流量重启持久化。未使用真实供应商令牌进行业务调用或生产负载测试。
