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

# 安装运行时依赖及构建工具（构建工具在 pip install 后清理）：
#   - ffmpeg：字幕检测、音频提取（findSystemFFmpeg 优先使用系统版本）
#   - git：pip install git+https:// 需要
#   - gcc + portaudio19-dev：pyaudio（whisperjav[cli] 依赖）编译需要
#   - libgomp1：ctranslate2 运行时依赖
#   - libsndfile1：soundfile 运行时依赖
#   - libportaudio2：pyaudio 运行时依赖（编译后仍需要共享库）
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        git \
        gcc \
        libc6-dev \
        portaudio19-dev \
        libgomp1 \
        libsndfile1 \
        libportaudio2 \
        ca-certificates

# 1) 先装 CPU-only PyTorch + torchaudio。必须在 WhisperJAV 之前从 CPU index 安装，
#    否则 openai-whisper / stable-ts 依赖会从默认 index 拉 GPU 版（需 libtorch_cuda.so）。
#    pip 发现 torch/torchaudio 已满足版本要求后不会重复安装。
RUN pip install --no-cache-dir torch torchaudio --index-url https://download.pytorch.org/whl/cpu

# 2) 安装 WhisperJAV[cli]（含 librosa、silero-vad、scikit-learn 等转录运行时依赖）。
#    [cli] extras 声明了 torch 依赖，但 pip 发现步骤 1 已安装的 CPU torch 满足版本要求，
#    不会重复下载 GPU 版本。
RUN pip install --no-cache-dir "whisperjav[cli] @ git+https://github.com/meizhong986/WhisperJAV.git"

# 3) 清理构建工具，减小镜像体积（保留 libportaudio2 运行时库）
RUN apt-get purge -y git gcc libc6-dev portaudio19-dev && apt-get autoremove -y \
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
