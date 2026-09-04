# ---- Build stage -----------------------------------------------------------
FROM golang:1.24-alpine AS build

WORKDIR /src

# Cache dependencies first for faster incremental builds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Version can be injected at build time: --build-arg VERSION=1.2.3
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/diglett .

# ---- Runtime stage ---------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Diglett" \
      org.opencontainers.image.description="Free, open-source web DNS lookup tool and API" \
      org.opencontainers.image.source="https://github.com/zollo/diglett" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/diglett /usr/local/bin/diglett

# The web UI and all assets are embedded in the binary, so no extra files are
# needed at runtime. Runs as the distroless nonroot user (uid 65532).
USER nonroot:nonroot
EXPOSE 8080

# Configuration is via environment variables (DIGLETT_*) or a mounted config
# file referenced by DIGLETT_CONFIG.
ENV DIGLETT_HOST=0.0.0.0 \
    DIGLETT_PORT=8080

ENTRYPOINT ["/usr/local/bin/diglett"]
