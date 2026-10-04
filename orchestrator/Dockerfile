FROM python:3.12-slim

# Install Docker CLI + Compose plugin
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl ca-certificates \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable" \
       > /etc/apt/sources.list.d/docker.list \
    && apt-get update && apt-get install -y --no-install-recommends docker-ce-cli docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY main.py ./

# Docker socket is mounted at runtime:
#   Linux/Mac: -v /var/run/docker.sock:/var/run/docker.sock
#   Windows Docker Desktop: -v //var/run/docker.sock://var/run/docker.sock
#     (or use DOCKER_HOST=tcp://host.docker.internal:2375 if TCP is enabled)
ENTRYPOINT ["python3", "/app/main.py"]
