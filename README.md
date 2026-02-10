# pushoo-chan-gover (Go 版)

🚀 使用 Go 重构的 [pushoo-chan](https://github.com/NyaMisty/pushoo-chan) 推送通知管理工具，完全兼容现有 `config.yaml` 格式。

## ✨ 特性

- 🔧 **纯 Go 实现**：使用标准库 `net/http`，无需 Node.js
- 🎨 **图形化配置界面**：友好的 Web UI，支持可视化编辑配置
- 🔐 **HTTP Basic Auth**：安全的认证机制
- 🔑 **推送 Token 保护**：可选的 API Token 验证，防止接口滥用
- 📊 **实时事件流**：SSE (Server-Sent Events) 支持
- 💾 **可选 SQLite**：使用 `modernc.org/sqlite`（纯 Go，无 CGO），支持维护与分页查询
- 🛡️ **反暴力破解**：内存 IP 封禁池 + 分片锁 + 自动清理
- 🐳 **Docker 支持**：一键部署，多架构支持
- 🌍 **多平台编译**：支持 Linux/Windows/ARM

## 📁 目录结构

```
gover/
├── main.go                    # 入口，启动 HTTP 服务
├── internal/
│   ├── httpapi/              # HTTP 路由与处理
│   ├── config/               # YAML 配置管理（热重载）
│   ├── push/                 # 推送服务 + provider
│   └── store/                # SQLite 存储（可选）
├── frontend/                 # 前端静态页面
│   ├── index.html           # 主页面
│   ├── app.js               # 前端逻辑
│   └── style.css            # 样式
├── Dockerfile               # Docker 镜像构建
├── docker-compose.yml       # Docker Compose 配置
├── build.sh                 # Linux/macOS 编译脚本
├── build.bat                # Windows 编译脚本
└── docker-update.sh         # Docker 一键更新脚本
```

## 🚀 快速开始

### 方式一：直接运行（开发/测试）

```bash
# 1. 安装依赖
cd gover
go mod tidy

# 2. 运行
go run . -addr :8084

# 3. 访问 Web 界面
# http://localhost:8084
```

**Windows 用户：**

```powershell
$env:GOPROXY="https://goproxy.cn,direct"
$env:GO111MODULE="on"
go mod tidy
go run . -addr :8084
```

### 方式二：编译后运行（生产环境）

```bash
# 编译
go build -o pushoo-chan-gover .

# 运行
./pushoo-chan-gover -addr :8084 -config ./config.yaml
```

### 方式三：Docker 部署（推荐）

详见 [Docker 部署指南](DOCKER_DEPLOYMENT.md)

```bash
# 1. 编译多平台二进制
./build.sh

# 2. 一键部署
./docker-update.sh

# 3. 访问应用
# http://localhost:8084
```

## ⚙️ 配置文件

### 配置文件优先级

1. 命令行参数：`-config <path>`
2. 环境变量：`PUSHOO_CONFIG_FILE`
3. 当前目录：`./config.yaml`
4. Docker 兼容：`./user_config/config.yaml`

### 配置示例

```yaml
channels:
  # tg channels
  - name: telegram
    type: telegram
    token: 233
  # bark channels
  - name: Bark
    type: bark
    token: "https://api.day.app/233/"
  # Dingtalk
  - name: dingtalk
    type: dingtalk
    token: https://oapi.dingtalk.com/robot/send?access_token=2333
  # Dingtalk
  - name: dingtalkmeeting
    type: dingtalk
    token: https://oapi.dingtalk.com/robot/send?access_token=233
  # wecom
  - name: wecom
    type: wecom
    token: 233
  # igot
  - name: igot
    type: igot
    token: 233
  # pushplus
  - name: pushplus
    type: pushplus
    token: 233
  # feishu
  - name: feishu
    type: feishu
    token: https://open.feishu.cn/open-apis/bot/v2/hook/2333

channel_groups:
  - name: all_channels
    use:
      - telegram
      - dingtalk

default_channel: all_channels

# Web 界面认证
auth:
  user: admin
  pass: yourpassword

# 推送 API Token 保护（可选）
push_token:
  enabled: false  # 设置为 true 启用 token 验证
  token: ""       # 留空则自动生成，或手动设置

# 可选：SQLite 数据库
sqlite:
  path: ./data/pushoo.db
  cleanup_days: 30
  cleanup_interval_hours: 24
  record_channel_messages: false

# 反暴力破解（内存 IP 封禁）
security:
  auth_fail_limit: 5
  auth_ban_minutes: 10
  token_fail_limit: 10
  token_ban_minutes: 10
  ip_ban_max_entries: 10000
  ip_ban_cleanup_seconds: 60
  ip_ban_idle_minutes: 60
```

### 图形化配置

访问 Web 界面后，可以使用图形化界面编辑配置：

- 📊 **图形化配置**：表单式编辑，支持添加/删除通道和通道组
- 📝 **YAML 编辑**：直接编辑 YAML 文本
- 🔄 **实时同步**：两种模式自动同步
- 💾 **一键保存**：保存后立即生效

## 🌐 API 接口

### 推送接口

#### 不启用 Token 验证（默认）

```bash
# GET 方式
curl "http://localhost:8084/send?text=Hello&desp=World&chan=telegram"

# POST 方式（表单）
curl -X POST http://localhost:8084/send \
  -d "text=Hello&desp=World&chan=telegram"

# POST 方式（JSON）
curl -X POST http://localhost:8084/send \
  -H "Content-Type: application/json" \
  -d '{"text":"Hello","desp":"World","chan":"telegram"}'
```

#### 启用 Token 验证后

当在配置文件中设置 `push_token.enabled: true` 后，所有推送请求必须包含 token 参数（支持 query / 表单 / JSON）：

```bash
# GET 方式（带 token）
curl "http://localhost:8084/send?token=YOUR_TOKEN&text=Hello&desp=World&chan=telegram"

# POST 方式（表单，带 token）
curl -X POST http://localhost:8084/send \
  -d "token=YOUR_TOKEN&text=Hello&desp=World&chan=telegram"

# POST 方式（JSON，token 在 URL）
curl -X POST "http://localhost:8084/send?token=YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"text":"Hello","desp":"World","chan":"telegram"}'

# POST 方式（JSON，token 在 JSON）
curl -X POST http://localhost:8084/send \
  -H "Content-Type: application/json" \
  -d '{"token":"YOUR_TOKEN","text":"Hello","desp":"World","chan":"telegram"}'
```

#### Bark 兼容接口（GET 可带标题/内容）

```
curl "http://localhost:8084/bark/telegram/Hello/World"

# Bark POST（表单）
curl -X POST http://localhost:8084/bark/telegram \
  -d "title=Hello&body=World"

# 启用 token 后
curl "http://localhost:8084/bark/telegram/Hello/World?token=YOUR_TOKEN"

# 启用 token 后（POST 表单）
curl -X POST http://localhost:8084/bark/telegram \
  -d "token=YOUR_TOKEN&title=Hello&body=World"
```

#### Bark v2 接口

```bash
# 不启用 token
curl -X POST http://localhost:8084/barkv2 \
  -H "Content-Type: application/json" \
  -d '{"device_key":"telegram","title":"Hello","body":"World"}'

# 启用 token 后
curl -X POST "http://localhost:8084/barkv2?token=YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"device_key":"telegram","title":"Hello","body":"World"}'
```

### 配置接口（需要 HTTP Basic Auth）

```bash
# 下载配置
curl -u admin:yourpassword http://localhost:8084/config/download

# 上传配置
curl -u admin:yourpassword -X POST http://localhost:8084/config/upload \
  -H "Content-Type: text/plain" \
  --data-binary @config.yaml
```

### 事件流接口（需要认证）

```bash
# SSE 事件流
curl -u admin:yourpassword http://localhost:8084/api/events
```

### 健康检查（公开）

```bash
curl http://localhost:8084/api/health
```

### 安全统计与趋势（需要认证，SQLite 启用后趋势可用）

```bash
# IP 封禁池统计
curl -u admin:yourpassword http://localhost:8084/api/security/ban_stats

# IP 封禁趋势（最近 24h / 7d）
curl -u admin:yourpassword http://localhost:8084/api/security/ban_trends
```

### SQLite 维护与分页（需要认证）

```bash
# 清理数据库（保留最近 N 天）
curl -u admin:yourpassword -X POST "http://localhost:8084/api/store/cleanup?keep_days=30"

# 压缩数据库体积
curl -u admin:yourpassword -X POST http://localhost:8084/api/store/compact

# 通知记录分页（默认 10 条/页）
curl -u admin:yourpassword "http://localhost:8084/api/store/notifications?page=1&page_size=10"
```

## 🔐 认证说明

### Web 界面认证

- **Web 界面**：需要 HTTP Basic Auth 认证
- **配置接口**：需要 HTTP Basic Auth 认证
- **事件流**：需要 HTTP Basic Auth 认证
- **健康检查**：公开，无需认证

首次运行时，默认认证信息：
- 用户名：`pushoo`
- 密码：`pushoo`

**⚠️ 重要：请立即修改默认密码！**

### 推送 API Token 保护

推送接口默认是公开的，但可以通过配置启用 Token 验证来保护接口不被滥用。

#### 启用 Token 验证

编辑 `config.yaml`：

```yaml
push_token:
  enabled: true   # 启用 token 验证
  token: ""       # 留空则自动生成 64 位随机 token
```

或手动设置 token：

```yaml
push_token:
  enabled: true
  token: "your_custom_token_here"
```

#### Token 自动生成

- 当 `enabled: true` 且 `token` 为空时，程序启动时会自动生成一个 64 位随机 token
- Token 会自动保存到配置文件中
- 只在首次启动或 token 为空时生成，不会覆盖已有 token

#### 使用 Token

启用后，所有推送请求必须包含 `token` 参数（支持 query / 表单 / JSON）：

```bash
# GET 请求
curl "http://localhost:8084/send?token=YOUR_TOKEN&text=Hello&desp=World"

# POST 请求（token 在 URL）
curl -X POST "http://localhost:8084/send?token=YOUR_TOKEN" \
  -d "text=Hello&desp=World"

# POST 请求（token 在表单）
curl -X POST http://localhost:8084/send \
  -d "token=YOUR_TOKEN&text=Hello&desp=World"

# POST 请求（token 在 JSON）
curl -X POST http://localhost:8084/send \
  -H "Content-Type: application/json" \
  -d '{"token":"YOUR_TOKEN","text":"Hello","desp":"World"}'
```

#### Token 验证失败

如果 token 错误或缺失，会返回 401 错误：

```json
{
  "error": "Invalid or missing push token",
  "msg": ["push token is required but not provided"]
}
```

#### 安全建议

1. **启用 Token 验证**：如果推送接口暴露在公网，强烈建议启用
2. **定期更换 Token**：定期修改 token 值
3. **使用 HTTPS**：配合反向代理使用 HTTPS
4. **限制访问**：使用防火墙限制访问 IP

详见 [认证说明文档](AUTHENTICATION.md)

## 💾 SQLite 存储（可选）

### 配置方式

**方式一：配置文件**

```yaml
sqlite:
  path: ./data/pushoo.db
  cleanup_days: 30
  cleanup_interval_hours: 24
  record_channel_messages: false
```

**方式二：命令行参数**

```bash
./pushoo-chan-gover -sqlite ./data/pushoo.db
```

### 特性

- ✅ WAL 模式：提升并发性能
- ✅ 外键约束：防止孤儿数据
- ✅ 事务写入：保证数据一致性
- ✅ 自动创建：首次运行自动初始化数据库
- ✅ 自动清理：按配置定期清理历史数据
- ✅ 维护接口：支持手动清理与压缩
- ✅ 记录开关：可选择是否保存通道级别的通知内容

### 数据表结构

```sql
-- 消息表
CREATE TABLE messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME,
  remote_addr TEXT,
  path TEXT,
  format TEXT,
  chan TEXT,
  title TEXT,
  content TEXT
);

-- 投递记录表
CREATE TABLE deliveries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id INTEGER,
  created_at DATETIME,
  channel_name TEXT,
  channel_type TEXT,
  status TEXT,
  detail TEXT,
  FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

-- IP 封禁池持久化（可选）
CREATE TABLE ip_bans (
  kind TEXT NOT NULL,
  ip TEXT NOT NULL,
  fail_count INTEGER NOT NULL,
  banned_until INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY(kind, ip)
);

-- 通道级通知记录（可选，受 record_channel_messages 控制）
CREATE TABLE channel_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  remote_addr TEXT,
  channel_name TEXT,
  channel_type TEXT,
  title TEXT,
  content TEXT,
  status TEXT,
  detail TEXT,
  FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
);
```

## 🔨 编译说明

### 编译单个平台

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o pushoo-chan-gover-linux-amd64 .

# Windows AMD64
GOOS=windows GOARCH=amd64 go build -o pushoo-chan-gover-windows-amd64.exe .

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o pushoo-chan-gover-linux-arm64 .

# Linux ARMv7
GOOS=linux GOARCH=arm GOARM=7 go build -o pushoo-chan-gover-linux-armv7 .
```

### 编译所有平台

**Linux/macOS：**

```bash
chmod +x build.sh
./build.sh
```

**Windows：**

```batch
build.bat
```

编译产物位于 `dist/` 目录。

## 🐳 Docker 部署

详见 [Docker 部署指南](DOCKER_DEPLOYMENT.md)

### 快速部署

```bash
# 1. 编译二进制
./build.sh

# 2. 构建并启动
docker-compose up -d

# 3. 查看日志
docker-compose logs -f
```

### 一键更新

```bash
./docker-update.sh
```

### 多架构支持

- ✅ Linux AMD64
- ✅ Linux ARM64 (树莓派 4/5)
- ✅ Linux ARMv7 (树莓派 2/3)

## 📊 Web 界面功能

### 配置管理

- **图形化配置**
  - 🔐 认证设置
  - 📢 通道配置（支持 8 种通道类型）
  - 👥 通道组配置
  - ⚙️ 其他设置（默认通道、SQLite 路径、自动清理、记录开关）
  - 🛡️ 反暴力破解配置（IP 封禁池参数）

- **YAML 编辑**
  - 直接编辑 YAML 文本
  - 语法高亮
  - 格式化功能

### 发送测试

- 选择通道
- 设置字符集
- 输入标题和内容
- 支持 GET/POST 方式

### 事件流

- 实时查看推送结果
- SSE 连接状态
- 事件日志

### 安全统计与趋势

- IP 封禁池统计（容量、样本 IP、估算内存）
- 最近 24h / 7d 趋势图（SQLite 启用后可用）

### 通知记录（SQLite）

- 分页查看通道级通知记录（默认 10 条/页）
- 仅 SQLite 启用时显示

## 🔧 命令行参数

```bash
./pushoo-chan-gover [选项]

选项：
  -addr string
        HTTP 监听地址 (默认 ":8084")
  -config string
        配置文件路径 (默认 "config.yaml")
  -sqlite string
        SQLite 数据库路径（可选）
  -frontend string
        前端文件目录 (默认 "frontend")
```

## 🌍 支持的通道类型

- ✅ Telegram
- ✅ Bark (iOS)
- ✅ 钉钉 (DingTalk)
- ✅ 企业微信 (WeCom)
- ✅ 企业微信机器人 (WeCom Bot)
- ✅ iGot
- ✅ PushPlus
- ✅ PushPlus HXTrip
- ✅ 飞书 (Feishu)
- ✅ ServerChan / ServerChain
- ✅ Qmsg
- ✅ PushDeer
- ✅ IFTTT
- ✅ Go-CQHTTP
- ✅ Atri
- ✅ Discord
- ✅ WxPusher
- ✅ Webhook
- ✅ Stub (测试)

## 📝 开发说明

### 项目架构

```
┌─────────────────────────────────────┐
│         Frontend (SPA)              │
│  - 图形化配置界面                    │
│  - YAML 编辑器                       │
│  - 发送测试                          │
│  - 事件流监控                        │
└─────────────────────────────────────┘
              ↓ HTTP
┌─────────────────────────────────────┐
│       HTTP API (net/http)           │
│  - 路由处理                          │
│  - Basic Auth 中间件                 │
│  - SSE 事件流                        │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│      Config Manager                 │
│  - YAML 解析                         │
│  - 热重载（3秒轮询）                  │
│  - 原子写入                          │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│      Push Service                   │
│  - 多通道推送                        │
│  - 通道组支持                        │
│  - 重试机制                          │
└─────────────────────────────────────┘
              ↓
┌─────────────────────────────────────┐
│      SQLite Store (可选)            │
│  - 消息历史                          │
│  - 投递记录                          │
│  - 外键约束                          │
└─────────────────────────────────────┘
```

### 添加新的通道类型

1. 在 `internal/push/providers/` 下创建新的 provider
2. 实现 `Provider` 接口
3. 在 `internal/push/service.go` 中注册

### 热重载机制

配置文件修改后，会在 3 秒内自动重载，无需重启服务。

## 🆘 故障排查

### 端口被占用

```bash
# Linux/macOS
lsof -i :8084

# Windows
netstat -ano | findstr :8084
```

### 配置文件错误

```bash
# 查看日志
./pushoo-chan-gover -addr :8084

# 验证 YAML 语法
cat config.yaml | python -c "import yaml, sys; yaml.safe_load(sys.stdin)"
```

### Docker 容器无法启动

```bash
# 查看日志
docker logs pushoo-chan-gover

# 检查配置
docker exec pushoo-chan-gover cat /app/data/config.yaml
```

## 📚 相关文档

- [Docker 部署指南](DOCKER_DEPLOYMENT.md)
- [认证说明](AUTHENTICATION.md)
- [初始化指南](INITIALIZATION_GUIDE.md)

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

AGPLv3

## 🔗 相关链接

- [原 TypeScript 版本](../README.md)
- [配置示例](config_example.yaml)
- [GitHub Repository](https://github.com/your-repo/pushoo-chan)
