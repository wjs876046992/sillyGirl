# Multi-arch Docker image for sillyGirl
# Uses node:24-alpine as base so we get Node.js 24 + npm + yarn out of the box.
#
# Usage:
#   docker run -d \
#     --name sillygirl \
#     -p 8080:8080 \
#     -v /host/data/path:/data \
#     ntwck/sillygirl:latest
#
# Binary at /sillyGirl (outside volume mount point).
# Data at /data — plugins, DB, config, all inside the volume.
#   /data/sillyGirl       — 软链到 /sillyGirl
#   /data/plugins/        — 插件目录
#   /data/node_modules/   — 插件 node 依赖
#   /data/.sillyplus/     — 数据目录（DB, 缓存等）

ARG TARGETARCH
ARG TARGETVARIANT

FROM node:24-alpine AS runtime

ARG TARGETARCH
ARG TARGETVARIANT

LABEL org.opencontainers.image.title="sillyGirl"
LABEL org.opencontainers.image.description="A wechat chatbot framework with Node.js plugin support"
LABEL org.opencontainers.image.url="https://github.com/ntwck/sillyGirl"
LABEL org.opencontainers.image.source="https://github.com/ntwck/sillyGirl"
LABEL org.opencontainers.image.licenses="MIT"
LABEL org.opencontainers.image.architecture="${TARGETARCH}/${TARGETVARIANT}"

# Add yarn 1.22.x (node:24-alpine ships with yarn 4.x by default)
RUN corepack enable \
    && corepack prepare yarn@1.22 --activate

# Copy binary to /sillyGirl (outside volume mount point)
COPY sillyGirl_linux_${TARGETARCH}${TARGETVARIANT} /sillyGirl
RUN chmod +x /sillyGirl

# Workdir is /data (persistent data volume)
WORKDIR /data

# Entrypoint: link binary into /data so ExecPath works,
# set data dir to /data, then exec.
RUN printf '#!/bin/sh\nln -sf /sillyGirl /data/sillyGirl\nexport SILLYGIRL_DATA_PATH=/data\nexec /sillyGirl "$@"\n' > /docker-entrypoint.sh && \
    chmod +x /docker-entrypoint.sh

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/docker-entrypoint.sh"]
