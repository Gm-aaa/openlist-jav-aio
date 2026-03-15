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
| 字幕翻译 | 调用 OpenAI 或 Ollama 将日语字幕翻译为目标语言 |
| 状态跟踪 | SQLite 记录每个文件每个步骤的完成状态，重启后自动续传 |
| 守护进程 | 定时轮询 + Webhook 触发，单工作协程，优雅关闭 |

---

## 快速开始

### 1. 下载或编译

```bash
git clone https://github.com/openlist-jav-aio/jav-aio
cd jav-aio
go build -o jav-aio .
```

> **注意：** ffmpeg 二进制为占位文件，需运行 `scripts/download-ffmpeg.sh` 下载真实二进制，或将系统 ffmpeg/ffprobe 放入 `$PATH`（subtitle 步骤需要）。

### 2. 创建配置文件

```bash
cp config.yaml.example config.yaml
# 编辑 config.yaml，至少填写 openlist.base_url、openlist.token、output.base_dir
```

### 3. 运行

```bash
# 一次性处理某个目录
./jav-aio run /jav/inbox

# 后台守护进程（推荐）
./jav-aio daemon
```

---

## 配置说明

配置文件默认读取当前目录的 `config.yaml`，可通过 `--config` 指定路径。

```yaml
openlist:
  base_url: "http://your-openlist:5244"   # OpenList 地址
  token: "your-api-token"                  # OpenList API Token
  scan_paths:
    - "/jav/inbox"                         # 要扫描的目录（可多个）
  scan_extensions:
    - ".mp4"
    - ".mkv"
    - ".avi"
  request_delay:
    min: "500ms"                           # 请求间最小延迟（防限流）
    max: "2s"                              # 请求间最大延迟
```

```yaml
output:
  base_dir: "/media/jav"                  # 输出根目录，每部影片生成子目录
```

```yaml
pipeline:
  poll_interval: "1h"                     # 守护进程轮询间隔
  steps:
    id_extract: true                       # 提取 JAV ID
    scrape: true                           # 抓取元数据
    strm: true                             # 生成 STRM 文件
    subtitle: true                         # 处理字幕
    translate: true                        # 翻译字幕
```

> 可将不需要的步骤设为 `false` 跳过。已完成的步骤不会重复执行。

```yaml
scraper:
  preferred_sources: ["javdb", "javbus"]  # 优先使用的元数据源
  language: "zh"                          # 元数据语言（zh/en/ja）
  cover: true                             # 是否下载封面图片
```

```yaml
subtitle:
  whisper_bin: "/usr/local/bin/whisperJAV"  # whisperJAV 可执行文件路径
  model: "medium"                            # whisper 模型（tiny/base/small/medium/large）
  language: "ja"                             # 音频语言（用于识别）
  ffmpeg_cache_dir: ""                       # ffmpeg 缓存目录，空=系统缓存
  keep_audio: false                          # 是否保留提取的音频文件
  keep_audio_max: 5                          # 最多保留几个音频文件（LRU 淘汰）
  audio_dir: ""                              # 音频文件存放目录，空=系统临时目录
```

```yaml
translate:
  target_language: "zh"                   # 翻译目标语言
  provider: "openai"                      # openai 或 ollama

  openai:
    api_key: "sk-..."
    base_url: "https://api.openai.com/v1" # 支持兼容 OpenAI 接口的第三方服务
    model: "gpt-4o-mini"

  ollama:
    base_url: "http://localhost:11434"
    model: "qwen2.5:7b"
```

```yaml
webhook:
  enabled: true
  port: 8080
  secret: "your-webhook-secret"           # HMAC-SHA256 签名密钥
```

```yaml
log:
  level: "info"                           # debug / info / warn / error
  format: "text"                          # text 或 json
  file: ""                                # 日志文件路径，空=输出到 stdout
```

```yaml
retry:
  max_attempts: 3
  base_delay: "2s"
  max_delay: "30s"
  jitter: true                            # 是否添加随机抖动
```

```yaml
state:
  db_path: "./jav-aio.db"                 # SQLite 状态数据库路径
```

---

## 命令参考

### `run` — 一次性处理目录

```bash
./jav-aio run /jav/inbox
./jav-aio run /jav/inbox --config /etc/jav-aio/config.yaml
```

列出指定 OpenList 路径下的所有视频文件，按流水线顺序处理每个文件。

### `daemon` — 守护进程模式

```bash
./jav-aio daemon
```

启动后：
1. 从数据库恢复上次未完成的任务
2. 按 `poll_interval` 定时扫描 `scan_paths`
3. 如果启用 Webhook，监听 HTTP 请求触发处理
4. 收到 `SIGTERM` / `Ctrl+C` 后优雅关闭

### `scrape` — 单独抓取元数据

```bash
./jav-aio scrape ABC-123
```

直接为指定 JAV ID 抓取元数据并写入 NFO，不经过流水线步骤检查。

### `strm` — 单独生成 STRM

```bash
./jav-aio strm /jav/inbox/ABC-123.mp4
```

为指定 OpenList 路径生成 `.strm` 文件。

### `subtitle` — 单独生成字幕

```bash
./jav-aio subtitle /jav/inbox/ABC-123.mp4
```

对指定文件执行字幕处理（外挂字幕检测 → 内嵌字幕提取 → whisperJAV 识别）。

---

## Webhook 接口

守护进程启用 Webhook 后，监听 `POST /webhook`。

**Headers：**

```
X-Signature: sha256=<hmac-sha256(secret, body)>
Content-Type: application/json
```

**OpenList 事件（通过文件路径触发）：**

```json
{
  "path": "/jav/inbox/ABC-123.mp4"
}
```

**外部触发（通过 JAV ID 触发，自动搜索路径）：**

```json
{
  "id": "ABC-123"
}
```

**响应：**

| 状态码 | 含义 |
|--------|------|
| 202 | 已接受，任务已入队 |
| 401 | 签名验证失败 |
| 400 | 请求格式错误 |

---

## 输出目录结构

每部影片在 `output.base_dir` 下生成独立子目录：

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

| 依赖 | 用途 | 必须 |
|------|------|------|
| [metatube-sdk-go](https://github.com/metatube-community/metatube-sdk-go) | 元数据抓取（已编译进二进制） | scrape 步骤必须 |
| ffmpeg / ffprobe | 字幕提取、音频提取 | subtitle 步骤必须 |
| [whisperJAV](https://github.com/meizhong986/WhisperJAV) | 语音识别生成字幕 | 无内嵌/外挂字幕时需要 |
| OpenAI 兼容 API 或 Ollama | 字幕翻译 | translate 步骤必须 |

### metatube-sdk-go

metatube-sdk-go 是 Go 模块依赖，已在编译时静态链接进二进制，**无需单独安装**。

它支持从以下数据源抓取元数据（通过 `scraper.preferred_sources` 配置优先顺序）：

| 数据源 | 配置值 |
|--------|--------|
| JavDB | `javdb` |
| JavBus | `javbus` |
| Tokyo Hot | `tokyohot` |
| Caribbeancom | `caribbeancom` |
| 1Pondo | `1pondo` |
| Heyzo | `heyzo` |
| FC2 | `fc2` |

未配置 `preferred_sources` 或匹配失败时，自动搜索所有可用数据源。

### 安装 ffmpeg

运行提供的脚本将 ffmpeg 下载到内嵌资产目录：

```bash
bash scripts/download-ffmpeg.sh
```

或直接将系统 ffmpeg/ffprobe 加入 `$PATH`，程序会自动检测。

### 安装 whisperJAV

```bash
git clone https://github.com/meizhong986/WhisperJAV
cd WhisperJAV
pip install -r requirements.txt
```

在配置中将 `subtitle.whisper_bin` 指向 `whisperJAV.py` 所在路径。

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

所有配置项均可通过环境变量覆盖，格式为大写加下划线。例如：

```bash
OPENLIST_TOKEN=my-token ./jav-aio daemon
```
