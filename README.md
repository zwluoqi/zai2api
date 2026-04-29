# zai2api

将 Z.AI 转换为 OpenAI 兼容 API 的代理服务。

[![Build](https://github.com/XxxXTeam/zai2api/actions/workflows/build.yml/badge.svg)](https://github.com/XxxXTeam/zai2api/actions/workflows/build.yml)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

## 功能特性

- **OpenAI 兼容 API** - 支持 `/v1/chat/completions` 和 `/v1/models` 端点
- **多模型支持** - 内置常用模型别名，并自动从云端同步最新模型列表
- **流式响应** - 支持 SSE 流式输出
- **工具调用** - 支持 Function Calling
- **多模态** - 支持图片输入
- **思考模式** - 支持 Thinking 模型的思考过程处理
- **Token 管理** - 自动管理和轮换 Token
- **遥测统计** - 请求计数、Token 统计、成功率等

## 快速开始

### 从 Release 下载

前往 [Releases](https://github.com/XxxXTeam/zai2api/releases) 下载对应平台的二进制文件。

### 从源码构建

```bash
git clone https://github.com/XxxXTeam/zai2api.git
cd zai2api
go build -o zai2api ./cmd/main.go
```

### 配置

复制配置文件并修改：

```bash
cp .env.example .env
```

编辑 `.env` 文件，设置必要的配置项：

```env
PORT=8000
AUTH_TOKEN=your-api-key
```

### 运行

```bash
./zai2api
```

服务将在 `http://localhost:8000` 启动。

## API 端点

| 端点 | 方法 | 描述 |
|------|------|------|
| `/` | GET | 服务状态和遥测数据 |
| `/v1/models` | GET | 获取可用模型列表 |
| `/v1/chat/completions` | POST | 聊天补全接口 |

## 配置项

| 配置项 | 默认值 | 描述 |
|--------|--------|------|
| `PORT` | 8000 | 服务端口 |
| `AUTH_TOKEN` | - | API 认证令牌（支持多个，逗号分隔） |
| `BACKUP_TOKEN` | - | 备用令牌（用于多模态） |
| `DEBUG_LOGGING` | false | 调试日志 |
| `TOOL_SUPPORT` | true | 工具调用支持 |
| `RETRY_COUNT` | 5 | 请求失败时的重试次数（不含首次请求） |
| `LOG_LEVEL` | info | 日志级别：debug/info/warn/error |

完整配置请参考 [.env.example](.env.example)

### Cloudflare Worker 转发

仓库包含 `worker.js`，可部署到 Cloudflare Workers 作为 `https://chat.z.ai` 的路径透传代理。使用脚本部署：

```bash
export CLOUDFLARE_API_TOKEN="your-cloudflare-api-token"
export CLOUDFLARE_ACCOUNT_ID="your-account-id"
export CLOUDFLARE_WORKER_NAME="your-worker-name"

python3 scripts/deploy-cloudflare-worker.py --enable-subdomain
```

也可以用命令行参数覆盖：

```bash
python3 scripts/deploy-cloudflare-worker.py \
  --api-token "your-cloudflare-api-token" \
  --account-id "your-account-id" \
  --script-name "your-worker-name" \
  --worker worker.js \
  --enable-subdomain
```

推荐将上游接口配置到 `config.json` 的 `api.endpoints`，也可以在管理页添加：

```json
{
  "api": {
    "endpoint": "https://your-worker.your-subdomain.workers.dev/api/v2/chat/completions",
    "endpoints": [
      "https://your-worker.your-subdomain.workers.dev/api/v2/chat/completions"
    ]
  }
}
```

## 使用示例

### cURL

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-key" \
  -d '{
    "model": "GLM-4.5",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-api-key",
    base_url="http://localhost:8000/v1"
)

response = client.chat.completions.create(
    model="GLM-4.5",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)
```

## 注册工具

注册工具位于 `cmd/register`，支持临时邮箱、Outlook/Hotmail 和 iCloud Hide My Email provider。Outlook 账号文件格式与 `deepseek-register` 一致：

```text
emailAddr----password----clientId----refreshToken
```

示例配置见 `cmd/register/config.example.json`。配置优先级为命令行参数、环境变量、配置文件、默认值：

```bash
go run ./cmd/register \
  --config cmd/register/config.example.json \
  --count 10 \
  --provider outlook \
  --outlook-file /path/to/outlook.txt \
  --proxy http://user:password@host:port
```

可用 provider 为 `temp`、`outlook`、`icloud`。`--count` / `run.count` / `ZAI_REGISTER_COUNT` 用于设置批量注册数量，成功的 token 会逐个追加到 `data/tokens.txt`。`run.proxy` 支持字符串或字符串数组；注册浏览器、Z.AI 注册 HTTP 请求、Microsoft OAuth 刷新和 iCloud API 请求会使用该代理配置。

滑块图片识别走 OpenAI 兼容接口，可通过 `vision.base_url`、`vision.api_key`、`vision.model`、`vision.slider_offset` 配置，也可用 `ZAI_VISION_BASE_URL`、`ZAI_VISION_API_KEY`、`ZAI_VISION_MODEL`、`ZAI_SLIDER_OFFSET` 环境变量覆盖。

注册浏览器默认使用 AdsPower 指纹环境，参数与参考项目保持一致：`browser.provider=adspower`、`browser.adspower_api`、`browser.adspower_group_id`、`browser.language`、`browser.timezone`、`browser.ua`。如需回退本地 Chrome，可加 `--browser local`。

## Docker 部署


GitHub Actions 会在 push 到 `main` 或推送 `v*` tag 时构建镜像并推送到 GHCR：

```text
ghcr.io/<owner>/<repo>:latest
ghcr.io/<owner>/<repo>:v1.0.0
ghcr.io/<owner>/<repo>:sha-xxxxxxx
```

服务器部署前准备 `.env`、`data/` 和注册相关文件：

```bash
mkdir -p data
touch data/tokens.txt data/tokens_invalid.txt outlook_used.txt outlook_bad.txt
cp .env.example .env
```

主服务最小挂载：

```yaml
volumes:
  - ./data:/app/data
```

注册器额外挂载：

```yaml
volumes:
  - ./data:/app/data
  - ./config.json:/app/config.json:ro
  - ./outlook.txt:/app/outlook.txt:ro
  - ./outlook_used.txt:/app/outlook_used.txt
  - ./outlook_bad.txt:/app/outlook_bad.txt
```

启动主服务：

```bash
ZAI2API_IMAGE=ghcr.io/<owner>/<repo>:latest docker compose up -d zai2api
```

运行一次 Outlook 注册：

```bash
ZAI2API_IMAGE=ghcr.io/<owner>/<repo>:latest docker compose run --rm register \
  zai2api-register --provider outlook --browser adspower --count 1
```

如果 AdsPower 跑在宿主机，Linux Docker 容器内的 `127.0.0.1` 指向容器自身。需要把 `ADSPOWER_API_URL` 或 `config.json` 里的 `browser.adspower_api` 改成可从容器访问的地址，例如 `http://host.docker.internal:50325`，必要时在 compose 里配置 `extra_hosts: ["host.docker.internal:host-gateway"]`。

## 管理面板

启动服务后访问 `http://localhost:8000/admin`。如果配置了 `ADMIN_TOKEN`，访问时需要加 `?token=...` 或请求头 `X-Admin-Token`。

当前面板支持：

- 请求统计：请求量、RPM、Token 消耗、成功率、有效账号数。
- 账号管理：查看账号/email/user_id/有效状态/使用次数，添加 token，立即验证 token。
- 失效池：`tokens_invalid.txt` 中的 token 会显示在账号列表里，保留失败原因，并支持重新启用回 `tokens.txt`。
- 系统设置：查看当前运行配置和注册相关环境变量。

## 支持的模型

- 启动后会从云端拉取最新模型列表，并自动补充到 `/v1/models`
- 当前模型列表可按基础模型分组理解，大多数基础模型会自动提供以下后缀变体：
  `-thinking`、`-search`、`-thinking-search`
- 当前已返回的基础模型包括：
  `GLM-4.5`
  `GLM-4.5-Search`
  `GLM-4.5-V`
  `GLM-4.5-Air`
  `GLM-4.6`
  `GLM-4.6-Thinking`
  `GLM-4.6-Search`
  `GLM-4.6-V`
  `GLM-4.7`
  `GLM-5`
  `GLM-5-Turbo`
  `GLM-5v-Turbo`
  `GLM-5.1`
  `glm-4.6v`
  `glm-4-flash`
  `glm-4-air-250414`
  `GLM-4.1V-Thinking-FlashX`

## 项目结构

```
zai2api/
├── cmd/
│   ├── main.go           # 主程序入口
│   └── register/         # Token 注册工具
├── internal/
│   ├── chat.go           # 聊天补全处理
│   ├── config.go         # 配置管理
│   ├── models.go         # 模型定义
│   ├── token_manager.go  # Token 管理
│   ├── tools.go          # 工具调用
│   └── ...
├── .env.example          # 配置示例
└── README.md
```

## 许可证

本项目采用 [GNU General Public License v3.0](LICENSE) 许可证。
