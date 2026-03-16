# =============================================================================
# Stage 1 – Go builder
# =============================================================================
FROM golang:1.26-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 如果 assets/linux_amd64 不存在（CI 中 gitignore 导致缺失），
# 从系统包获取 ffmpeg/ffprobe 并放入 embed 路径，保证 go:embed 编译成功。
RUN if [ ! -f internal/ffmpeg/assets/linux_amd64/ffmpeg ]; then \
        apt-get update && apt-get install -y --no-install-recommends ffmpeg && \
        mkdir -p internal/ffmpeg/assets/linux_amd64 && \
        cp "$(which ffmpeg)"  internal/ffmpeg/assets/linux_amd64/ffmpeg && \
        cp "$(which ffprobe)" internal/ffmpeg/assets/linux_amd64/ffprobe && \
        rm -rf /var/lib/apt/lists/*; \
    fi

RUN CGO_ENABLED=0 GOOS=linux go build -o /jav-aio ./cmd

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
RUN git clone --depth=1 https://github.com/meizhong986/WhisperJAV.git /opt/WhisperJAV \
    && pip install --no-cache-dir -r /opt/WhisperJAV/requirements.txt \
    && printf '#!/bin/sh\nexec python /opt/WhisperJAV/whisperjav.py "$@"\n' \
       > /app/bin/whisperjav \
    && chmod +x /app/bin/whisperjav

# 复制编译好的二进制（内嵌了 ffmpeg linux_amd64）
COPY --from=builder /jav-aio /usr/local/bin/jav-aio

WORKDIR /app

# HuggingFace 模型缓存目录（挂载命名卷）
ENV HF_HOME=/app/hf-cache

EXPOSE 8080

ENTRYPOINT ["jav-aio"]
CMD ["daemon", "--config", "/app/config.yaml"]
