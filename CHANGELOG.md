# Changelog

所有重要变更均记录在此文件中，格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [0.1.4] - 2026-03-17

### 修复

- **Dockerfile WhisperJAV 安装** — 改用 `whisperjav[cli]` extras 安装，包含全部转录运行时依赖（silero-vad、scikit-learn 等）；之前仅手动补装 soundfile/librosa，运行时缺包报 ImportError
- **Dockerfile 缺少 libsndfile1** — soundfile 运行时依赖的系统库未安装，导致 `OSError: cannot load library`
- **Docker whisper_bin 路径错误** — `config.docker.yaml` 指向 `/app/bin/whisperjav`（不存在），pip 实际安装到 `/usr/local/bin`；改为 `whisperjav` 通过 PATH 查找
- **WhisperJAV CLI 参数与源码不匹配** — 按实际 argparse 定义校正：恢复 `--no-signature`、`--no-progress`、`--accept-cpu-mode`；移除不存在的 `--cpu-only`、`--cpu-threads`
- **`run` 命令冗余 API 调用** — 未检查 `IsComplete()`，对已完成文件仍调用 `/api/fs/get`；1000 个已完成文件 = 1000 次浪费的 API 调用
- **重启恢复丢失 sign** — daemon 重启后重新入队的任务 sign 为空，强制调用 `/api/fs/get` 获取新签名；现在 sign 缓存在 SQLite，重启后直接复用
- **DB 与文件系统状态不同步** — DB 标记 `strm_done=1` 或 `subtitle_done=1` 但文件被删除时，pipeline 不会重新生成；新增孤立状态检测，文件缺失时自动重置标记
- **STRM 无条件覆写** — `strm.Generate()` 每次都写入磁盘，即使内容未变；改为幂等写入，内容相同时跳过
- **`config.docker.yaml` 幽灵配置** — 移除无效的 `id_extract: true`（Go 代码中无对应字段，被静默忽略）

### 新增

- **`cpu_only` 配置项** — 控制是否传递 `--accept-cpu-mode` 给 WhisperJAV，Docker 默认 true，有 GPU 环境设 false
- **SQLite `sign` 列** — 缓存 OpenList 下载签名，减少重启后的 API 调用（含旧库自动迁移）

### 移除

- **`cpu_threads` 配置项** — WhisperJAV CLI 不支持此参数，移除以避免运行时报错

---

## [0.1.3] - 2026-03-16

### 修复

- **Dockerfile `go build ./cmd`** — `cmd/` 包声明为 `package cmd`（非 `package main`），`go build ./cmd` 生成的是归档文件（`.a`）而非 ELF 可执行二进制，导致容器内 `exec format error`；改为 `go build .` 从根目录 `main.go` 构建
- **Dockerfile 多平台 GOARCH** — 多平台构建（amd64 + arm64）时未设置 `GOARCH`，Go 默认使用宿主机架构，导致 arm64 变体编译出错误架构二进制
- **Dockerfile ffmpeg 冗余** — 构建阶段不再复制 ~80MB 的真实 ffmpeg 二进制，改用空占位文件满足 `go:embed`，运行时使用 apt 安装的系统 ffmpeg
- **ffmpeg 动态链接缺库** — 内嵌的 ffmpeg 为构建阶段的动态链接版本，运行时缺少 `libavdevice.so.59` 等共享库（`exit status 127`）；新增 `findSystemFFmpeg()` 优先使用系统 apt 安装的 ffmpeg，仅在系统无 ffmpeg 时回退到内嵌解压（Windows / 本地开发）
- **docker-compose 数据库挂载** — `./jav-aio.db:/app/data/jav-aio.db` 在宿主机文件不存在时被 Docker 创建为目录；改为挂载整个 `./data:/app/data`
- **`whisper/download.go`** — `DownloadModel` 将 stdout/stderr 捕获到 buffer，用户看不到模型下载进度（1.5~3GB 等待无反馈）；改为直接输出到终端
- **`subtitle/subtitle.go`** — `Process` 方法缺少 `p.ffmpeg` nil 检查，ffmpeg 初始化失败时直接调用会 panic
- **`subtitle/subtitle.go`** — `HasExternalSubtitle` 未校验 SRT 内容，被中断的 ffmpeg 留下的截断空文件会被误判为有效字幕；新增 `" --> "` 时间码校验，自动删除无效 SRT
- **`ffmpeg/runner.go` / `scraper/nfo.go` / `scraper/metatube.go` / `cmd/app.go`** — 文件写入非原子操作，进程中断时产生截断文件；改为写入 `.tmp` + `os.Rename` 原子替换
- **`state/db.go`** — SQLite 并发写入冲突（`database is locked`）；启用 WAL 模式 + `busy_timeout=5000ms`；`Get` 返回 `ErrNotFound` 哨兵错误替代裸 `sql.ErrNoRows`
- **`pipeline/pipeline.go`** — `DB.Get` 未区分"记录不存在"和"数据库错误"，真实 DB 故障被静默忽略；新增 `ErrNotFound` 判断，真实错误中断流水线
- **`pipeline/pipeline.go`** — 步骤成功时无条件清除 `ErrorMsg`，导致后续步骤成功覆盖前序步骤的错误信息；改为 `clearError()` 仅清除本步骤设置的错误
- **`pipeline/pipeline.go`** — 各步骤间缺少 `ctx.Err()` 检查，用户取消后仍继续执行后续步骤
- **`cmd/daemon.go`** — 关停时 `close(taskQueue)` 先于后台 goroutine 退出，导致 `send on closed channel` panic；新增 `bgWg.Wait()` 保证 goroutine 全部退出后再关闭
- **`cmd/daemon.go`** — 向 `taskQueue` 发送未检查 `ctx.Done()`，取消后阻塞或 panic；新增 `trySend()` 带 context 感知
- **`webhook/server.go`** — HMAC 签名比较使用 `strings.EqualFold`（非恒定时间），存在时序攻击风险；改为 `hmac.Equal`
- **`scheduler/scheduler.go`** — `fn` panic 导致整个调度器崩溃；新增 `safeRun()` panic recovery
- **`openlist/client.go`** — 空页面未终止分页循环，OpenList 返回空 `Content` 时无限请求；新增 `len(data.Content) == 0` 守卫
- **`openlist/client.go`** — `GetFileURL` 未编码路径中的空格和 CJK 字符，含特殊字符的文件名生成无效 URL；新增 `encodePath()` 逐段百分号编码
- **`llm/deeplx.go`** — 信号量获取未检查 `ctx.Done()`，取消后 goroutine 泄漏；新增 select 双路监听

### 优化

- **LLM 翻译性能** — 仅发送对话文本（去除时间码和序号），减少 40–60% token 消耗；批次大小从 50 提升至 100；`temperature: 0` 确保确定性输出
- **OpenAI prompt caching** — 固定 system message 与 user message 分离，命中 OpenAI 自动 prompt 缓存，降低重复请求成本
- **Markdown 围栏剥离** — LLM 返回 ` ```srt ... ``` ` 包裹内容时自动剥离，避免翻译结果解析失败

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
- **WhisperJAV 调优参数** — 新增 `sensitivity`、`compute_type` 配置项
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
- WhisperJAV 可配置参数：`sensitivity`、`compute_type`
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

