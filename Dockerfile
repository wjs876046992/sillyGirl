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
# Binary at /sillyGirl (outside mount).
# Data at /data/plugins, /data/data, etc. (persistent volume).
# Entrypoint ensures /data/sillyGirl always exists before startup.

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

# Workdir is /data (persistent data volume), entrypoint restores binary
WORKDIR /data

# Entrypoint script: link /sillyGirl into /data so ExecPath works,
# then exec the binary with all passed args.
RUN printf '#!/bin/sh\nln -sf /sillyGirl /data/sillyGirl\nexec /sillyGirl "$@"\n' > /docker-entrypoint.sh && \
    chmod +x /docker-entrypoint.sh

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/docker-entrypoint.sh"]
