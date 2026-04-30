# Multi-arch Docker image for sillyGirl
# Uses node:24-alpine as base so we get Node.js 24 + npm + yarn out of the box.
#
# Usage:
#   docker run -d \
#     --name sillygirl \
#     -p 8080:8080 \
#     -v /host/data/path:/app \
#     ntwck/sillygirl:latest
#
# Everything lives under /app — binary, plugins, language, config, data.
# Mount /app for full persistence.

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

# Everything lives under /app — binary, plugins, language, config, data
WORKDIR /app

COPY sillyGirl_linux_${TARGETARCH}${TARGETVARIANT} /app/sillyGirl

RUN chmod +x /app/sillyGirl

EXPOSE 8080

VOLUME ["/app"]

ENTRYPOINT ["/app/sillyGirl"]
