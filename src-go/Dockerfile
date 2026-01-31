# musicd Docker build container
# Builds the musicd binary with all required dependencies

FROM golang:1.24-bookworm AS builder

# Install build dependencies for audio (ALSA) and D-Bus (MPRIS)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libasound2-dev \
    libdbus-1-dev \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION}" \
    -o /musicd \
    ./cmd/musicd

# Final stage - just the binary
FROM scratch
COPY --from=builder /musicd /musicd
ENTRYPOINT ["/musicd"]
