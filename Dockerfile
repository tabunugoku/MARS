# syntax=docker/dockerfile:1
#
# ComiNavi: one static Go binary with the web UI embedded, on distroless.
# The node and Go stages run on the build host and cross-compile, so a
# multi-arch build never runs npm or the Go toolchain under emulation.

# --- web UI ---------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# copy-dist.mjs empties this folder (keeping .gitkeep) before copying dist in.
RUN mkdir -p /src/server/internal/webui/dist && npm run build:embed

# --- server ---------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
COPY --from=web /src/server/internal/webui/dist ./internal/webui/dist
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/cominavi ./cmd/cominavi \
 && mkdir -p /out/data

# --- runtime --------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot
COPY --from=server /out/cominavi /usr/local/bin/cominavi
# Owned by the image's nonroot user (65532) so a named volume starts writable.
COPY --from=server --chown=65532:65532 /out/data /data
ENV COMINAVI_DATA=/data \
    COMINAVI_PORT=25600
EXPOSE 25600
VOLUME /data
USER nonroot:nonroot
# The image has no shell or curl, so the binary probes itself.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/usr/local/bin/cominavi", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/cominavi"]
