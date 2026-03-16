# jav-aio

自动化 JAV 处理流水线，对接 OpenList 文件源，完成以下完整工作流：

**JAV ID 提取 → 元数据抓取 → STRM 生成 → 字幕提取/识别 → 字幕翻译**

支持守护进程模式（定时轮询 + Webhook），可长期运行。

---

## 功能概览

| 功能 | 说明 |
|------|------|
| OpenList 扫描 | 自动列出指定目录下的视频文件，支持递归、分页、延迟控制 |
| JAV ID 提取 | 正则识别 `ABC-123`、`FC2-PPV-123456` 等格式 |
| 元数据抓取 | 通过 metatube 从 javdb/javbus 等源获取标题、演员、封面，输出 NFO |
| STRM 生成 | 生成 `.strm` 文件，供 Emby/Jellyfin 直接串流 |
| 字幕处理 | 优先使用外挂字幕，其次提取内嵌字幕，最后用 whisperJAV 语音识别 |
| 字幕翻译 | 支持 OpenAI 兼容接口 / Ollama / DeepLX，并发分块翻译 |
| 状态跟踪 | SQLite 记录每个文件每个步骤的完成状态，重启后自动续传 |
| 守护进程 | 定时轮询 + Webhook 触发，单工作协程，优雅关闭 |
| 主动通知 | 翻译完成后向配置的 URL 发送 Webhook 通知 |

---

## 快速开始

### 方式一：Docker（推荐）

```bash
# 1. 复制配置文件
cp config.yaml.example config.docker.yaml
# 编辑 config.docker.yaml，填写 openlist.base_url、token 等

# 2. 启动
docker compose up -d --build

# 3. 预下载 Whisper 模型（只需执行一次）
docker compose exec jav-aio jav-aio model download medium --config /app/config.yaml
```

> Docker 镜像自动安装 WhisperJAV、ffmpeg，无需手动配置。
> Whisper 模型缓存在命名卷中，容器重建后无需重新下载。

### 方式二：本地编译

```bash
git clone https://github.com/openlist-jav-aio/jav-aio
cd jav-aio
go build -o jav-aio ./cmd
```

> **注意：** ffmpeg/ffprobe 已内嵌进二进制，首次运行时自动解压到缓存目录，无需手动安装。

```bash
# 复制并编辑配置
cp config.yaml.example config.yaml

# 一次性处理某个目录
./jav-aio run /jav/inbox

# 守护进程模式（推荐）
./jav-aio daemon
```

---

## 配置说明

配置文件默认读取当前目录的 `config.yaml`，可通过 `--config` 指定路径。完整示例见 [`config.yaml.example`](config.yaml.example)。

```yaml
openlist:
  base_url: "http://your-openlist:5244"   # OpenList 地址
  token: "your-api-token"                  # OpenList API Token
  scan_paths:
    - "/jav/inbox"
  scan_extensions: [".mp4", ".mkv", ".avi"]
  request_delay:
    min: "500ms"
    max: "2s"
  min_file_size: "500MB"                   # 小于此值的文件跳过（过滤视频广告）
```

```yaml
subtitle:
  whisper_bin: "/usr/local/bin/whisperjav" # whisperJAV 可执行文件路径
  python_bin: "python3"                    # 用于 model download 命令
  model: "medium"                          # tiny/base/small/medium/large-v3
  language: "ja"
  sensitivity: ""          # 幻觉过滤灵敏度：aggressive/conservative/balanced
  compute_type: ""         # 量化精度：int8_float32（CPU 推荐）/ float16（GPU 推荐）
  cpu_threads: 0           # CPU 线程数，0=默认(1)，建议设为 vCPU 数量
  ffmpeg_cache_dir: ""     # ffmpeg 解压缓存目录，空=系统缓存
  keep_audio: false
  keep_audio_max: 5
  audio_dir: ""
```

```yaml
translate:
  target_language: "zh"
  provider: "deeplx"       # deeplx（推荐）/ openai / ollama
  max_tokens: 0            # LLM 输出 token 上限，0=API 默认

  deeplx:
    base_url: "http://localhost:1188"
    source_lang: "JA"

  openai:
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o-mini"

  ollama:
    base_url: "http://localhost:11434"
    model: "qwen2.5:7b"
```

```yaml
# 翻译完成后主动推送通知
notify:
  enabled: false
  url: "https://your-webhook-endpoint"
  # headers:
  #   Authorization: "Bearer token"
```

通知 Payload：
```json
{
  "event": "translate_done",
  "jav_id": "ABC-123",
  "path": "/jav/inbox/ABC-123.mp4",
  "srt_path": "/media/jav/ABC-123/ABC-123.srt",
  "timestamp": "2026-03-16T00:00:00Z"
}
```

```yaml
webhook:                   # 接收外部触发
  enabled: true
  port: 8080
  secret: "your-secret"

log:
  level: "info"            # debug / info / warn / error
  format: "text"           # text 或 json
  file: ""                 # 空=stdout

state:
  db_path: "./jav-aio.db"
```

---

## 命令参考

### `daemon` — 守护进程模式

```bash
./jav-aio daemon
./jav-aio daemon --config /etc/jav-aio/config.yaml
```

启动后：恢复未完成任务 → 定时扫描 → 监听 Webhook → 收到信号后优雅关闭。

### `run` — 一次性处理目录

```bash
./jav-aio run /jav/inbox
```

### `model download` — 预下载 Whisper 模型

```bash
./jav-aio model download           # 使用 config.yaml 中配置的模型
./jav-aio model download large-v3  # 指定模型
```

支持的模型：`tiny` / `base` / `small` / `medium` / `large-v3` / `large-v3-turbo`，以及完整 HuggingFace Repo ID（如 `litagin/anime-whisper`）。

### `scrape` — 单独抓取元数据

```bash
./jav-aio scrape ABC-123
```

### `strm` — 单独生成 STRM

```bash
./jav-aio strm /jav/inbox/ABC-123.mp4
```

### `subtitle` — 单独生成字幕

```bash
./jav-aio subtitle /jav/inbox/ABC-123.mp4
```

---

## Webhook 接口

守护进程启用 Webhook 后，监听 `POST /webhook`。

**Headers：**
```
X-Signature: sha256=<hmac-sha256(secret, body)>
Content-Type: application/json
```

**通过文件路径触发：**
```json
{ "path": "/jav/inbox/ABC-123.mp4" }
```

**通过 JAV ID 触发：**
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
/media/jav/
└── ABC-123/
    ├── ABC-123.strm          # 串流文件
    ├── ABC-123.nfo           # 元数据（Emby/Jellyfin 识别）
    ├── ABC-123.jpg           # 封面图片
    ├── ABC-123.srt           # 日语字幕
    └── ABC-123.zh.srt        # 翻译后字幕
```

---

## 依赖说明

| 依赖 | 用途 | 安装方式 |
|------|------|----------|
| [metatube-sdk-go](https://github.com/metatube-community/metatube-sdk-go) | 元数据抓取 | 已编译进二进制，无需安装 |
| ffmpeg / ffprobe | 字幕提取、音频提取 | 已内嵌进二进制，首次运行自动解压 |
| [WhisperJAV](https://github.com/meizhong986/WhisperJAV) | 语音识别生成字幕 | `pip install whisperjav` |
| OpenAI 兼容 API / Ollama / DeepLX | 字幕翻译 | 按需配置 |

### 安装 WhisperJAV

```bash
pip install whisperjav faster-whisper
```

在配置中将 `subtitle.whisper_bin` 指向可执行文件（通常为 `/usr/local/bin/whisperjav`）。

### DeepLX 部署

```bash
docker run -d -p 1188:1188 zu1k/deeplx
```

速度极快（秒级），免费，翻译质量接近 DeepL Pro。

---

## 作为系统服务运行

### systemd（Linux）

```ini
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

## 环境变量

所有配置项均可通过环境变量覆盖（Viper 自动映射）：

```bash
OPENLIST_TOKEN=my-token ./jav-aio daemon
```
