# =============================================================================
# Stage 1 – Go builder
# =============================================================================
FROM golang:1.26-bookworm AS builder

# BuildKit 在多平台构建时自动设置 TARGETARCH（amd64 / arm64 等）
# 必须声明后才能在 RUN 中使用
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# go:embed 要求 assets/linux_amd64 和 assets/windows_amd64 在编译时存在。
# CI checkout 时两个目录均因 .gitignore 缺失，用空占位文件满足编译。
# Docker 运行时使用系统 apt 安装的 ffmpeg（findSystemFFmpeg），不使用内嵌版本。
RUN mkdir -p internal/ffmpeg/assets/linux_amd64 \
    && touch internal/ffmpeg/assets/linux_amd64/ffmpeg \
    && touch internal/ffmpeg/assets/linux_amd64/ffprobe \
    && mkdir -p internal/ffmpeg/assets/windows_amd64 \
    && touch internal/ffmpeg/assets/windows_amd64/ffmpeg.exe \
    && touch internal/ffmpeg/assets/windows_amd64/ffprobe.exe

# 显式传入 GOARCH，确保交叉编译时生成正确平台的二进制
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o /jav-aio .

# =============================================================================
# Stage 2 – Python + WhisperJAV runtime
# =============================================================================
FROM python:3.11-slim-bookworm

# 固定目录结构（不依赖挂载路径）：
#   /app/ffmpeg/          —— ffmpeg_cache_dir（本镜像使用系统 ffmpeg，此目录闲置）
#   /app/data/output/     —— 输出文件 (output.base_dir)
#   /app/data/audio/      —— 音频缓存 (subtitle.audio_dir)
#   /app/data/jav-aio.db  —— 状态数据库 (state.db_path)
#   /app/config.yaml      —— 运行时挂载（只读）
RUN mkdir -p /app/ffmpeg /app/data/output /app/data/audio

# 安装运行时依赖并在同一层清理，减小镜像体积：
#   - ffmpeg：字幕检测、音频提取（findSystemFFmpeg 优先使用系统版本）
#   - git：pip install git+https:// 需要（安装完后删除）
#   - libgomp1：ctranslate2 运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        git \
        libgomp1 \
    && rm -rf /var/lib/apt/lists/*

# 1) 先装 CPU-only PyTorch（~280MB）。必须在 WhisperJAV 之前，
#    否则 openai-whisper 依赖会从默认 index 拉完整 GPU 版（~800MB）。
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu

# 2) 安装 WhisperJAV。它的核心依赖包含 openai-whisper（~100MB，且我们只用
#    faster-whisper），用 --no-deps 跳过自动依赖解析，然后只安装实际需要的包。
RUN pip install --no-cache-dir --no-deps \
        "whisperjav @ git+https://github.com/meizhong986/WhisperJAV.git"

# 3) 手动安装 WhisperJAV 运行时实际需要的依赖（跳过 openai-whisper）。
#    stable-ts 提供语音识别（内部调用 faster-whisper）；soundfile/librosa 用于
#    音频加载；其余为 WhisperJAV CLI 必需的轻量包。
#    注意：ffmpeg-python 不需要，WhisperJAV 直接通过 subprocess 调用系统 ffmpeg。
RUN pip install --no-cache-dir \
        stable-ts \
        faster-whisper \
        huggingface_hub \
        soundfile \
        librosa \
        pysrt \
        srt \
        tqdm \
        colorama \
        requests \
        regex \
        more-itertools \
        pydantic \
        PyYAML \
        jsonschema

# 4) 清理构建工具，减小镜像体积
RUN apt-get purge -y git && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/* /root/.cache/pip

# 复制编译好的二进制并确保可执行权限
COPY --from=builder /jav-aio /usr/local/bin/jav-aio
RUN chmod +x /usr/local/bin/jav-aio

WORKDIR /app

# HuggingFace 模型缓存目录（挂载命名卷）
ENV HF_HOME=/app/hf-cache

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/jav-aio"]
CMD ["daemon", "--config", "/app/config.yaml"]
