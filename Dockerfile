# Build stage
FROM golang:1.22.12-alpine AS builder
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
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*
RUN python -m venv /opt/venv \
    && /opt/venv/bin/pip install --no-cache-dir \
    baml \
    instructor \
    outlines \
    json-repair \
    git+https://github.com/opt-nc/yamlfixer
ENV PATH="/opt/venv/bin:${PATH}"

RUN npm install -g @openai/codex @anthropic-ai/claude-code
RUN curl -fsSL https://opencode.ai/install | bash -s -- --no-modify-path
ENV PATH="/root/.opencode/bin:${PATH}"
WORKDIR /home/app
COPY schemas/review-result.schema.json /opt/go-go-agent-action/schema/review-result.schema.json
COPY --from=builder /out/agent-action /usr/local/bin/agent-action
ENTRYPOINT ["/usr/local/bin/agent-action"]
