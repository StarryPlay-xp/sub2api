# 本机流量套餐监控：部署说明
最后更新：2026-11-02

此功能只统计运行 sub2api 的 Linux 主机。采集在应用进程内完成，无独立安装程序、设备注册、上报地址或服务商接口。服务流量、WireGuard 流量和本机网卡流量是不同统计口径，不能相加。

## 新部署

仓库的四份 `docker-compose*.yml` 已默认把宿主机 `/sys` 只读挂到 `/host/sys`，不需要额外采集程序。请使用 Linux Docker Engine；Docker Desktop 中的 Linux 虚拟机不是桌面系统本身。

自行维护 Compose 时，可把 [compose.host-traffic.yml](compose.host-traffic.yml) 放在部署目录并加入完整 Compose 文件列表。该文件与默认 Compose 的同目标挂载合并，不需要重复添加。

请挂载整个 `/sys`，不要只挂 `/sys/class/net`：其中的相对符号链接需要能访问 `/sys/devices`。不要加 `:z` / `:Z` 给系统目录重新打标签。不要挂宿主机 `/proc`，不要添加特权模式、Docker socket、host PID 或 host network。

## 已有 Docker Compose + image-updater：一次性开启

这一步会短暂重建 **sub2api 应用容器**，中断当时的请求。数据库和 Redis 不重建，不删除卷。建议在业务低峰操作，并先确认现有备份可用。

1. 把本仓库的 `deploy/compose.host-traffic.yml` 放到实际部署目录，文件名保持 `compose.host-traffic.yml`。使用与现有 Compose 文件一致的受控所有者和权限；更新器要求配置文件由 root 管理且不能被普通用户改写。先确认后台没有进行中的升级任务；有任务时等待结束，不要中途停止服务。确认空闲后执行 `sudo systemctl stop sub2api-updater`，停止更新器，避免维护期间有人触发升级。
2. 备份 `/etc/sub2api-updater.json`，在其中 `compose_files` 数组的末尾追加新附加文件的**绝对路径**。保留原有主文件、更新服务附加文件、WireGuard 附加文件及其他已有覆盖文件，保持原顺序。不要覆盖成示例配置。不要把自动生成的 `compose.image-version.json` 写入该数组。
3. 以 root 在本机运行下面的检查与重建命令。它从现有更新器配置读取目录、项目名、环境文件和完整文件列表，并保留更新器生成的版本覆盖文件。脚本要求更新器已停止；挂载检查和更新器预检查通过后，才重新启动更新器加载新配置。更新器没有配置热重载，只修改 JSON 不会让运行中的进程使用新文件列表。

```bash
sudo python3 - <<'PY'
import json
import os
from pathlib import Path
import subprocess

if os.geteuid() != 0:
    raise SystemExit('请以 root 执行')
service = subprocess.run(['systemctl', 'is-active', 'sub2api-updater'],
                         capture_output=True, text=True, check=False)
if service.stdout.strip() != 'inactive':
    raise SystemExit('请先确认无升级任务并停止更新器；状态异常时停止操作')
cfg = json.loads(Path('/etc/sub2api-updater.json').read_text())
root = Path(cfg['project_directory'])
files = [Path(p) for p in cfg['compose_files']]
extra = root / 'compose.host-traffic.yml'
if extra not in files or not extra.is_file():
    raise SystemExit('先放置附加文件并把它加入 compose_files')
if not Path('/sys/class/net').is_dir():
    raise SystemExit('本机不是受支持的 Linux 宿主机')
for path in files + [Path(cfg['env_file'])]:
    if not path.is_file():
        raise SystemExit('配置文件缺失，停止操作')
version = root / 'compose.image-version.json'
if version.exists():
    files.append(version)
args = ['docker', '--host', 'unix:///var/run/docker.sock', 'compose',
        '--project-directory', str(root), '-p', cfg['project_name'],
        '--env-file', cfg['env_file']]
for path in files:
    args += ['-f', str(path)]
# 不打印展开后的配置，其中可能包含密码。
subprocess.run(args + ['config', '--quiet'], check=True)
subprocess.run(args + ['up', '-d', '--no-deps', '--force-recreate', 'sub2api'], check=True)
container = subprocess.check_output(args + ['ps', '-q', 'sub2api'], text=True).strip()
if not container:
    raise SystemExit('应用容器未启动，停止并检查本地日志')
info = json.loads(subprocess.check_output(
    ['docker', '--host', 'unix:///var/run/docker.sock', 'inspect', container], text=True))[0]
if not any(m['Destination'] == '/host/sys' and m['Source'] == '/sys'
           and m['Type'] == 'bind' and not m['RW'] for m in info['Mounts']):
    raise SystemExit('挂载校验失败，不要继续执行后台升级')
# --check 读取新配置并检查完整挂载；会初始化更新器状态目录，但不升级镜像。
# 检查失败则保持更新器停止，不会继续启动或自动回滚应用。
subprocess.run(['python3', '/usr/local/lib/sub2api-updater/updater.py',
                '--config', '/etc/sub2api-updater.json', '--check'], check=True)
subprocess.run(['systemctl', 'start', 'sub2api-updater'], check=True)
subprocess.run(['systemctl', 'is-active', '--quiet', 'sub2api-updater'], check=True)
print('挂载已检查，更新器已重新加载配置；等待应用健康后检查采集状态。')
PY
```

4. 等应用恢复健康，再等到下一个整分钟。采集应从 `starting` 进入 `ok`，来源为 `host_mount`。首次没有确定的默认出口时，在套餐表单里选计费网卡；Docker 下 `default_route` 是 `null`，这属于正常行为。
5. 确认脚本的预检查和更新器启动均成功，后台显示更新服务可用，且应用恢复健康后再允许后台升级。以后的普通镜像升级会保留挂载，无需重复操作。更新器会拒绝“配置声明了新挂载、旧容器却未重建”以及“进程仍使用旧文件列表、容器已有新挂载”两种状态。

没有使用 image-updater 时，也要把新附加文件加入日常使用的全部 Compose 命令，并用 `up -d --no-deps --force-recreate sub2api` 重建应用。不要使用 `down -v`、删除数据目录或换一套项目名。

若失败，停止继续升级并保持更新器停止。需要恢复时，先恢复备份的配置文件和原完整 Compose 文件列表，再只重建应用容器；应用恢复健康后，用恢复后的配置运行上述 `updater.py --config /etc/sub2api-updater.json --check`，通过后执行 `sudo systemctl start sub2api-updater` 重新加载配置。预检查失败时不要启动更新器。这里不涉及数据库版本回退。

## 二进制 + systemd

Linux 二进制直接读取 `/sys/class/net`、`/proc/net/route`、`/proc/net/ipv6_route`、`/proc/uptime` 和 `/proc/sys/kernel/random/boot_id`。现有 `deploy/sub2api.service` 的 `ProtectSystem=strict` 限制写入，`ProtectHome` 和 `PrivateTmp` 不遮蔽上述读取位置，静态检查不需要修改加固配置。

应用内置 IANA 时区数据库，无需另装系统时区包。非 Linux 返回 `unsupported`。

## 状态与统计含义

- `needs_host_mount`：Docker 没有宿主机挂载，不会退回容器网卡。
- `starting`：等待第一次整分钟持久化读数；数据库正常时通常不超过一分钟。
- `stale`：最近已保存读数超过三分钟。
- `error`：读取或保存失败。接口只返回简短原因码，具体环境在本机排查。
- `exact`：对选定网卡观测完整，不代表服务商最终账单已核实。
- `estimate`：使用服务商数值补齐或覆盖，校准后没有明显缺口。
- `lower`：用量至少达到显示值；预警仍会触发，不会自动限速或断网。

邮件是否能送达还取决于运维告警邮件开关、收件人、邮件服务、严重级别过滤、静默和频率限制；`email_enabled` 仅表示运维邮件开关。

## 尚需授权后的实机验收

本次开发未连接或操作服务器。以下仍需在用户授权的 Linux 主机上验证：实际宿主机网卡读取、容器的 boot_id/uptime 与宿主机的一致性、Docker inspect 对 `/sys` 挂载的实际表示、更新器真实升级后的复核、宿主机重启连续性，以及邮件送达。特别是容器运行时可能遮蔽 boot_id，遇到 `boot_identity_unavailable` 时应先排查，不能把容器数据伪装成主机数据。
