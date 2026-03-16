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

# embed.go 要求 assets/linux_amd64 和 assets/windows_amd64 在编译时同时存在。
# CI checkout 时两个目录均因 .gitignore 缺失，需在此补全：
#   - linux_amd64：从系统包获取真实 ffmpeg/ffprobe 二进制（运行时使用）
#   - windows_amd64：创建空占位文件（仅满足 go:embed 编译要求，Linux 镜像不使用）
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg \
    && mkdir -p internal/ffmpeg/assets/linux_amd64 \
    && cp "$(which ffmpeg)"  internal/ffmpeg/assets/linux_amd64/ffmpeg \
    && cp "$(which ffprobe)" internal/ffmpeg/assets/linux_amd64/ffprobe \
    && mkdir -p internal/ffmpeg/assets/windows_amd64 \
    && touch internal/ffmpeg/assets/windows_amd64/ffmpeg.exe \
    && touch internal/ffmpeg/assets/windows_amd64/ffprobe.exe \
    && rm -rf /var/lib/apt/lists/*

# 显式传入 GOARCH，确保交叉编译时生成正确平台的二进制
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o /jav-aio ./cmd

# =============================================================================
# Stage 2 – Python + WhisperJAV runtime
# =============================================================================
FROM python:3.11-slim-bookworm

# 固定目录结构（不依赖挂载路径）：
#   /app/bin/whisperjav   —— WhisperJAV 可执行文件
#   /app/ffmpeg/          —— 内嵌 ffmpeg 首次运行自动解压到此
#   /app/data/output/     —— 输出文件 (output.base_dir)
#   /app/data/audio/      —— 音频缓存 (subtitle.audio_dir)
#   /app/data/jav-aio.db  —— 状态数据库 (state.db_path)
#   /app/config.yaml      —— 运行时挂载（只读）
RUN mkdir -p /app/bin /app/ffmpeg /app/data/output /app/data/audio

# 安装 git（克隆 WhisperJAV 源码需要）和 libgomp1（ctranslate2 运行时依赖）
RUN apt-get update && apt-get install -y --no-install-recommends \
        git \
        libgomp1 \
    && rm -rf /var/lib/apt/lists/*

# 安装 CPU-only PyTorch（单独一层，cache 命中率高）
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu

# 安装 faster-whisper 及 HuggingFace 工具库
RUN pip install --no-cache-dir faster-whisper huggingface_hub

# 从 GitHub 安装 WhisperJAV（PyPI 上不可用），并在固定路径创建 wrapper 脚本
# 过滤掉 requirements.txt 中的 torch 行（已通过 CPU-only index 安装，避免版本冲突）
RUN git clone --depth=1 https://github.com/meizhong986/WhisperJAV.git /opt/WhisperJAV \
    && grep -iv "torch" /opt/WhisperJAV/requirements.txt \
       | pip install --no-cache-dir -r /dev/stdin \
    && printf '#!/bin/sh\nexec python /opt/WhisperJAV/whisperjav.py "$@"\n' \
       > /app/bin/whisperjav \
    && chmod +x /app/bin/whisperjav

# 复制编译好的二进制并确保可执行权限
COPY --from=builder /jav-aio /usr/local/bin/jav-aio
RUN chmod +x /usr/local/bin/jav-aio

WORKDIR /app

# HuggingFace 模型缓存目录（挂载命名卷）
ENV HF_HOME=/app/hf-cache

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/jav-aio"]
CMD ["daemon", "--config", "/app/config.yaml"]
