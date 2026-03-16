# jav-aio

[![Build & Push Docker Image](https://github.com/Gm-aaa/openlist-jav-aio/actions/workflows/docker.yml/badge.svg)](https://github.com/Gm-aaa/openlist-jav-aio/actions/workflows/docker.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

自动化 JAV 处理流水线，对接 [OpenList](https://github.com/OpenListTeam/OpenList) 文件源，完成以下完整工作流：

**JAV ID 提取 → 元数据抓取 → STRM 生成 → 字幕提取/识别 → 字幕翻译**

支持守护进程模式（定时轮询 + Webhook），可长期无人值守运行。

---

## 功能概览

| 功能 | 说明 |
|------|------|
| OpenList 扫描 | 自动列出指定目录下的视频文件，支持递归、分页、请求延迟控制 |
| JAV ID 提取 | 正则识别 `ABC-123`、`FC2-PPV-123456` 等格式 |
| 元数据抓取 | 从 javdb/javbus 等来源获取标题、演员、封面，输出 NFO 供 Emby/Jellyfin 使用 |
| STRM 生成 | 生成 `.strm` 串流文件，Emby/Jellyfin 无需下载即可播放 |
| 字幕处理 | 三级检测：外挂字幕 → 内嵌字幕流 → WhisperJAV 语音识别 |
| 字幕翻译 | 支持 DeepLX / OpenAI 兼容接口 / Ollama，并发分块翻译 |
| 状态跟踪 | SQLite 记录每个步骤完成状态，重启后自动续传 |
| 守护进程 | 定时轮询 + Webhook 触发，优雅关闭 |
| 主动通知 | 翻译完成后向指定 URL 推送 Webhook 通知 |

---

## 部署方式

### 方式一：Docker（推荐）

Docker 镜像内置 WhisperJAV 和 ffmpeg，开箱即用，无需手动配置 Python 环境。

**1. 拉取镜像**

```bash
docker pull ghcr.io/gm-aaa/openlist-jav-aio:latest
```

**2. 获取 docker-compose 和配置模板**

```bash
# 下载 docker-compose.yml
curl -O https://raw.githubusercontent.com/Gm-aaa/openlist-jav-aio/master/docker-compose.yml

# 下载配置模板并重命名为 Docker 专用配置
curl -o config.docker.yaml https://raw.githubusercontent.com/Gm-aaa/openlist-jav-aio/master/config.yaml.example
```

**3. 编辑配置文件**

编辑 `config.docker.yaml`，至少填写以下字段（路径已为容器内固定路径，无需修改）：

```yaml
openlist:
  base_url: "http://your-openlist:5244"
  token: "your-api-token"
  scan_paths:
    - "/your/media/path"

translate:
  provider: "deeplx"   # 或 openai / ollama
  openai:
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o-mini"
```

容器内固定路径（`config.docker.yaml` 中保持不变）：

```yaml
subtitle:
  whisper_bin: "/app/bin/whisperjav"
  ffmpeg_cache_dir: "/app/ffmpeg"
  audio_dir: "/app/data/audio"
output:
  base_dir: "/app/data/output"
state:
  db_path: "/app/data/jav-aio.db"
```

**4. 启动服务**

```bash
docker compose up -d
```

**5. 预下载 Whisper 语音识别模型**（首次部署必做，模型约 1.5GB，缓存在命名卷中，之后重启无需重复）

```bash
docker compose exec jav-aio jav-aio model download medium --config /app/config.yaml
```

**6. 查看运行日志**

```bash
docker compose logs -f
```

---

### 方式二：本地编译运行

**环境要求：** Go 1.26+、Python 3.9+（用于 WhisperJAV）

**1. 编译**

```bash
git clone https://github.com/Gm-aaa/openlist-jav-aio
cd openlist-jav-aio
go build -o jav-aio ./cmd
```

> ffmpeg/ffprobe 已内嵌进二进制，首次运行时自动解压到缓存目录，**无需手动安装 ffmpeg**。

**2. 安装 WhisperJAV**

```bash
# 安装 CPU-only PyTorch（无 GPU 时推荐，体积更小）
pip install torch --index-url https://download.pytorch.org/whl/cpu

# 安装 WhisperJAV 及依赖
pip install faster-whisper huggingface_hub
git clone https://github.com/meizhong986/WhisperJAV.git
cd WhisperJAV && grep -iv "torch" requirements.txt | pip install -r /dev/stdin
```

**3. 下载语音识别模型**

```bash
./jav-aio model download medium
# 可选模型：tiny / base / small / medium / large-v3
```

**4. 创建配置文件并运行**

```bash
cp config.yaml.example config.yaml
# 编辑 config.yaml

# 守护进程模式（推荐）
./jav-aio daemon

# 一次性处理某个目录
./jav-aio run /jav/inbox
```

---

## 配置说明

完整配置示例见 [`config.yaml.example`](config.yaml.example)。

### OpenList 连接

```yaml
openlist:
  base_url: "http://your-openlist:5244"  # OpenList 服务地址
  token: "your-api-token"                # 管理后台 → 个人资料 → API 令牌
  scan_paths:
    - "/115/jav"                         # 要扫描的目录（可配多个）
  min_file_size: "500MB"                 # 小于此值的文件跳过，用于过滤视频广告
  request_delay:
    min: "500ms"                         # 每次 API 请求后的随机延迟，防止触发限流
    max: "2s"
```

### 字幕识别（WhisperJAV）

```yaml
subtitle:
  whisper_bin: "/usr/local/bin/whisperjav"  # WhisperJAV 可执行文件路径
  model: "medium"       # 模型大小，影响识别准确率和速度，见下方说明
  language: "ja"        # 视频语言，日语 JAV 填 ja
  sensitivity: ""       # 幻觉过滤灵敏度，见下方说明
  compute_type: ""      # 推理精度，见下方说明
  cpu_threads: 0        # CPU 线程数，0 = 默认单线程，多核服务器建议填 vCPU 数量
```

**模型选择（`model`）**

| 模型 | 速度 | 准确率 | 内存占用 | 推荐场景 |
|------|------|--------|----------|----------|
| `tiny` | 极快 | 低 | ~400MB | 仅测试 |
| `base` | 快 | 一般 | ~600MB | 快速验证 |
| `small` | 中等 | 较好 | ~900MB | 资源有限时 |
| `medium` | 慢 | 好 | ~1.5GB | **默认推荐** |
| `large-v3` | 最慢 | 最佳 | ~3GB | 追求最高质量 |

也支持填写 HuggingFace Repo ID，如 `litagin/anime-whisper`（针对日语动画/成人内容优化）。

**幻觉过滤灵敏度（`sensitivity`）**

WhisperJAV 内置过滤器，用于去除语音识别中常见的幻觉内容（无意义重复、乱码等）。

| 值 | 效果 | 适用场景 |
|----|------|----------|
| `""` 留空 | 使用 WhisperJAV 默认设置 | 一般情况 |
| `"conservative"` | 过滤力度弱，保留更多内容，可能包含少量幻觉 | 内容丰富、对话较多的视频 |
| `"balanced"` | 过滤力度适中 | 通用 |
| `"aggressive"` | 过滤力度强，更干净，但可能误删部分真实字幕 | 背景噪音大、或字幕块数为 0 时尝试 |

> **提示：** 如果识别结果字幕为空（0 块），通常是过滤器过于激进，可尝试将 `sensitivity` 改为 `"conservative"`。

**推理精度（`compute_type`）**

影响推理速度和精度，在 CPU 服务器上尤为重要。

| 值 | 速度 | 精度 | 推荐场景 |
|----|------|------|----------|
| `""` 留空 | — | — | 使用 WhisperJAV 默认 |
| `"int8"` | 最快 | 略低 | 对速度要求高、精度要求低 |
| `"int8_float32"` | 快 | 接近原始 | **CPU 服务器推荐**，速度 2-3x，精度几乎不损失 |
| `"float16"` | 快 | 高 | **GPU 推荐** |
| `"float32"` | 慢 | 最高 | 精度要求极高时 |

---

### 字幕翻译

```yaml
translate:
  target_language: "zh"   # 翻译目标语言（zh/en/ko 等）
  provider: "deeplx"      # 翻译服务，见下方对比
  max_tokens: 0           # LLM 输出 token 上限，0 = 使用 API 默认值
```

**翻译提供商对比**

| 提供商 | 速度 | 质量 | 费用 | 适用场景 |
|--------|------|------|------|----------|
| `deeplx` | ⚡ 极快（秒级） | 良好 | 免费 | 日常使用推荐 |
| `openai` | 慢（分钟级） | 最佳 | 按 token 计费 | 对翻译质量要求高 |
| `ollama` | 慢 | 中等 | 免费（本地） | 离线环境 |

```yaml
  # DeepLX（需自行部署，docker run -d -p 1188:1188 owo-network/deeplx）
  deeplx:
    base_url: "http://localhost:1188"
    source_lang: "JA"   # 源语言，留空自动检测

  # OpenAI 兼容接口（支持 GPT / Kimi / DeepSeek / 火山方舟等）
  openai:
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o-mini"

  # Ollama 本地大模型
  ollama:
    base_url: "http://localhost:11434"
    model: "qwen2.5:7b"
```

---

### 翻译完成通知（Notify）

翻译成功后向指定地址发送 POST 通知，可对接飞书机器人、企业微信、Bark、n8n 等。

```yaml
notify:
  enabled: true
  url: "https://your-webhook-endpoint"
  headers:                         # 可选，用于认证
    Authorization: "Bearer token"
```

通知内容（JSON）：

```json
{
  "event":     "translate_done",
  "jav_id":    "ABC-123",
  "path":      "/115/jav/ABC-123.mp4",
  "srt_path":  "/app/data/output/ABC-123/ABC-123.srt",
  "timestamp": "2026-03-16T00:00:00Z"
}
```

---

### 其他配置

```yaml
pipeline:
  poll_interval: "1h"    # 守护进程扫描间隔
  steps:
    scrape: true         # 元数据抓取（生成 NFO + 封面）
    strm: true           # 生成 .strm 串流文件
    subtitle: true       # 字幕识别
    translate: true      # 字幕翻译
    # 不需要的步骤设为 false 即可跳过；已完成的步骤重启后不会重复执行

webhook:                 # 接收外部 HTTP 触发（传入式 Webhook）
  enabled: true
  port: 8080
  secret: "your-secret" # HMAC-SHA256 签名密钥，留空则不验签

log:
  level: "info"          # debug / info / warn / error
  format: "text"         # text（人类可读）/ json（适合日志收集）
  file: ""               # 日志文件路径，留空输出到 stdout

state:
  db_path: "./jav-aio.db"
```

---

## 命令参考

```bash
# 守护进程模式（推荐生产使用）
./jav-aio daemon

# 一次性扫描并处理目录
./jav-aio run /jav/inbox

# 预下载 Whisper 模型
./jav-aio model download           # 使用 config 中配置的模型
./jav-aio model download large-v3  # 指定模型

# 单独执行某步骤（调试用）
./jav-aio scrape ABC-123           # 抓取元数据
./jav-aio strm /jav/ABC-123.mp4    # 生成 STRM 文件
./jav-aio subtitle /jav/ABC-123.mp4 # 生成字幕

# 指定配置文件
./jav-aio daemon --config /etc/jav-aio/config.yaml
```

---

## 传入 Webhook 接口

守护进程启用 `webhook.enabled: true` 后监听 `POST /webhook`，用于外部系统主动触发处理。

**请求头：**
```
Content-Type: application/json
X-Signature: sha256=<hmac-sha256(secret, body)>   # secret 为空时可省略
```

**通过文件路径触发：**
```json
{ "path": "/jav/inbox/ABC-123.mp4" }
```

**通过 JAV ID 触发（自动搜索路径）：**
```json
{ "id": "ABC-123" }
```

| 状态码 | 含义 |
|--------|------|
| 202 | 已接受，任务已入队 |
| 401 | 签名验证失败 |
| 400 | 请求格式错误 |

---

## 输出目录结构

```
/app/data/output/
└── ABC-123/
    ├── ABC-123.strm       # Emby/Jellyfin 串流文件
    ├── ABC-123.nfo        # 元数据（标题、演员、评分等）
    ├── ABC-123.jpg        # 封面图片
    ├── ABC-123.srt        # 日语原始字幕
    └── ABC-123.zh.srt     # 翻译后中文字幕
```

---

## 作为 systemd 服务运行

```ini
# /etc/systemd/system/jav-aio.service
[Unit]
Description=JAV AIO Daemon
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/jav-aio
ExecStart=/opt/jav-aio/jav-aio daemon --config /opt/jav-aio/config.yaml
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now jav-aio
sudo journalctl -u jav-aio -f
```

---

## 开源声明

本项目基于以下优秀的开源项目构建，向各位作者致谢：

| 项目 | 作者 | 用途 | 协议 |
|------|------|------|------|
| [WhisperJAV](https://github.com/meizhong986/WhisperJAV) | [@meizhong986](https://github.com/meizhong986) | JAV 语音识别生成字幕 | MIT |
| [faster-whisper](https://github.com/SYSTRAN/faster-whisper) | [SYSTRAN](https://github.com/SYSTRAN) | 高效 Whisper 推理引擎 | MIT |
| [metatube-sdk-go](https://github.com/metatube-community/metatube-sdk-go) | [metatube-community](https://github.com/metatube-community) | JAV 元数据抓取 | Apache-2.0 |
| [DeepLX](https://github.com/OwO-Network/DeepLX) | [@OwO-Network](https://github.com/OwO-Network) | 免费 DeepL 翻译接口 | MIT |
| [FFmpeg](https://ffmpeg.org) | FFmpeg 项目组 | 音视频处理 | LGPL-2.1 |
| [cobra](https://github.com/spf13/cobra) | [@spf13](https://github.com/spf13) | CLI 框架 | Apache-2.0 |
| [viper](https://github.com/spf13/viper) | [@spf13](https://github.com/spf13) | 配置管理 | MIT |
| [glebarez/sqlite](https://github.com/glebarez/sqlite) | [@glebarez](https://github.com/glebarez) | 纯 Go SQLite 驱动 | MIT |

---

## License

[MIT](LICENSE) © 2026 Gm-aaa
