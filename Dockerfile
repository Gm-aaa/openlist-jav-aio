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

# 安装 CPU-only PyTorch（单独一层，cache 命中率高）
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu

# 安装 WhisperJAV 及依赖，软链到固定路径
RUN pip install --no-cache-dir whisperjav faster-whisper huggingface_hub \
    && ln -s "$(which whisperjav)" /app/bin/whisperjav

# 复制编译好的二进制（内嵌了 ffmpeg linux_amd64）
COPY --from=builder /jav-aio /usr/local/bin/jav-aio

WORKDIR /app

# HuggingFace 模型缓存目录（挂载命名卷）
ENV HF_HOME=/app/hf-cache

EXPOSE 8080

ENTRYPOINT ["jav-aio"]
CMD ["daemon", "--config", "/app/config.yaml"]
