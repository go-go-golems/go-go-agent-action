# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /out/agent-action ./cmd/agent-action

# Run stage
FROM python:3.12-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    ca-certificates \
    bash \
    jq \
    sqlite3 \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*
RUN python -m venv /opt/venv \
    && /opt/venv/bin/pip install --no-cache-dir \
    baml \
    instructor \
    outlines \
    json-repair \
    git+https://github.com/opt-nc/yamlfixer
ENV PATH="/opt/venv/bin:${PATH}"
WORKDIR /home/app
COPY --from=builder /out/agent-action /usr/local/bin/agent-action
ENTRYPOINT ["/usr/local/bin/agent-action"]
