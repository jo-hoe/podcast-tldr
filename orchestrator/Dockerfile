FROM python:3.12-slim

# Install Docker CLI so the orchestrator can call `docker compose run`
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

# The orchestrator needs the Docker socket and the compose files from the repo root.
# Mount: /var/run/docker.sock, /app/mount/work, and the repo root as /repo.
ENTRYPOINT ["python3", "/app/main.py"]
