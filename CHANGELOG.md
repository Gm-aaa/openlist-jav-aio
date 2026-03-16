# Changelog

所有重要变更均记录在此文件中，格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [0.1.1] - 2026-03-16

### 修复

- **`llm/openai.go`** — `json.Marshal` / `http.NewRequestWithContext` 错误被忽略，`req` 为 nil 时触发 panic；HTTP 状态码检查由 `!= 200` 改为 `>= 300`，兼容 201 等合法响应
- **`llm/ollama.go`** — 同上 panic 风险修复；新增 HTTP 状态码检查，Ollama 返回 5xx 时报告明确错误而非 JSON 解码失败
- **`llm/srt.go`** — LLM 返回乱码时 `SplitSRT` 结果为空，对应 chunk 字幕静默丢失；现返回明确错误
- **`whisper/runner.go`** — `os.Stat` 无法识别 `$PATH` 中的命令（如 Docker 内 `"whisperjav"`），改用 `exec.LookPath` 兼容绝对路径与 PATH 名称

### 新增

- **Docker 支持** — 多阶段 Dockerfile，自动安装 WhisperJAV（pip）及 ffmpeg；CI 环境下自动从系统包补全 embed 所需的 ffmpeg 二进制
- **docker-compose.yml** — 固定容器内路径，挂载命名卷缓存 Whisper 模型
- **GitHub Actions** — 自动构建并推送镜像到 ghcr.io，PR 仅构建不推送，tag 触发语义化版本标签
- **主动通知（notify）** — 字幕翻译完成后向配置的 URL 发送 Webhook 通知，支持自定义请求头
- **DeepLX 翻译提供商** — 新增 `deeplx` provider，20 并发逐块翻译
- **并发 LLM 翻译** — OpenAI / Ollama 均改为 50 块/批、10 并发分块翻译
- **`jav-aio model download`** — 从 HuggingFace 预下载 faster-whisper 模型
- **WhisperJAV 调优参数** — 新增 `sensitivity`、`compute_type`、`cpu_threads` 配置项
- **`translate.max_tokens`** 配置项

---

## [0.1.0] - 2026-03-16

首个正式版本，完整实现从视频入库到字幕翻译的自动化流水线。

### 新增

**核心流水线**
- 五步流水线：ID 提取 → 元数据抓取 → STRM 生成 → 字幕识别 → 字幕翻译
- SQLite 状态数据库，每步完成后记录，重启后自动跳过已完成步骤
- 守护进程模式 (`daemon`)，定时扫描 OpenList 目录并处理新文件

**字幕识别**
- 三级字幕检测：外挂字幕 → 内嵌字幕流 → WhisperJAV 语音识别
- 内嵌 ffmpeg/ffprobe 二进制（Linux/Windows），无需手动安装
- WhisperJAV 可配置参数：`sensitivity`、`compute_type`、`cpu_threads`
- `jav-aio model download` 命令，自动从 HuggingFace 下载 faster-whisper 模型
- CRLF 归一化，修复 Windows 下 WhisperJAV 输出的 SRT 解析问题

**字幕翻译**

- 支持三种翻译提供商：OpenAI 兼容接口 / Ollama / DeepLX
- 并发分块翻译（50 块/批，默认 10 并发），大幅提升翻译速度
- 可配置 `max_tokens` 上限
- 翻译完成后可通过 `notify` 配置主动推送 Webhook 通知

**OpenList 集成**
- 支持 OpenList API Token 认证
- 文件大小过滤（`min_file_size`），跳过视频广告
- 可配置请求延迟，防止触发限流

**元数据抓取**
- 基于 metatube-sdk-go，支持 javdb、javbus 等多个来源
- 生成 NFO 文件和封面图片
- FC2-PPV 系列支持

**其他**
- 接收外部 Webhook 触发单个文件处理（HMAC-SHA256 验签）
- 指数退避重试策略
- 结构化日志（text/json 格式，支持输出到文件）
- Dockerfile + docker-compose，自动安装 WhisperJAV 及依赖
- GitHub Actions 自动构建并推送镜像到 ghcr.io

---

