# Stage 1: Build Frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app
RUN npm install -g pnpm

COPY package.json pnpm-lock.yaml* ./
RUN pnpm install

COPY frontend ./frontend
RUN pnpm dlx @tailwindcss/cli -i ./frontend/assets/styles/input.css -o ./frontend/assets/styles/tailwind.css --minify

# Stage 2: Build Backend
FROM golang:1.24-alpine AS backend-builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o fast-stream-bot ./cmd/fsb

# Stage 3: Final Image
FROM alpine:latest
WORKDIR /app

# Install necessary runtime dependencies
RUN apk --no-cache add ca-certificates mailcap tzdata

# Copy binary
COPY --from=backend-builder /app/fast-stream-bot .

# Copy frontend assets (HTML templates and static files)
COPY frontend ./frontend

# Copy built CSS (overwriting source if present)
COPY --from=frontend-builder /app/frontend/assets/styles/tailwind.css ./frontend/assets/styles/tailwind.css

# Expose port (default 8000)
EXPOSE 8000

CMD ["./fast-stream-bot"]
