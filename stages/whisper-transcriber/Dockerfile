# Runtime image for the transcribe stage.
#
# Two-stage build: install Python deps in the full image (where build tools are
# available), then copy into the slim CUDA runtime.
#
# Base image choice:
#   nvidia/cuda:12.1.0-cudnn8-runtime-ubuntu22.04
#   - CUDA 12.1 + cuDNN 8 runtime libs — what CTranslate2 (faster-whisper) needs.
#   - Ubuntu 22.04 ships Python 3.10 via apt; we layer 3.12 via deadsnakes PPA.
#   - Pinned to 12.1.0: CTranslate2 wheels are compiled against CUDA 12.x; bumping
#     the minor is usually safe but bumping the major (13.x) may break ABI.
#     (Dependabot is configured to ignore cuda image bumps for this reason.)
#
# CPU fallback: set device: cpu in config/transcribe.yaml — the same image works,
# CUDA libs are simply unused.

# --- build stage: install Python deps ---
FROM nvidia/cuda:12.1.0-cudnn8-runtime-ubuntu22.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# Install Python 3.12 + pip from deadsnakes PPA.
RUN apt-get update && apt-get install -y --no-install-recommends \
        software-properties-common \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update && apt-get install -y --no-install-recommends \
        python3.12 \
        python3.12-venv \
        python3.12-dev \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Use a venv so the runtime stage can copy a single clean tree.
RUN python3.12 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

WORKDIR /build
COPY pyproject.toml ./
COPY transcribe ./transcribe
COPY main.py ./

RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir .

# --- runtime stage ---
FROM nvidia/cuda:12.1.0-cudnn8-runtime-ubuntu22.04

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

# Minimal Python 3.12 runtime (no build tools).
RUN apt-get update && apt-get install -y --no-install-recommends \
        software-properties-common \
    && add-apt-repository ppa:deadsnakes/ppa \
    && apt-get update && apt-get install -y --no-install-recommends \
        python3.12 \
    && rm -rf /var/lib/apt/lists/*

# Copy venv from builder.
COPY --from=builder /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

WORKDIR /app
COPY transcribe ./transcribe
COPY main.py ./

# Create an unprivileged user and the mount points it will read/write.
RUN useradd --create-home --uid 65532 appuser \
    && mkdir -p /app/mount/work /app/mount/models \
    && chown -R appuser:appuser /app

USER appuser

# Where faster-whisper caches downloaded models (see config modelCachePath).
ENV HF_HOME=/app/mount/models

ENTRYPOINT ["python3.12", "/app/main.py"]
