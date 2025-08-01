FROM golang:alpine AS builder

WORKDIR /app

COPY . .

RUN apk add --no-cache \
    gcc \
    musl-dev \
    libwebp-dev \
    git

RUN apk add --no-cache \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    freetype-dev \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    xvfb

RUN go mod download

RUN CGO_ENABLED=1 GOOS=linux go build -o main ./cmd

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache \
    libwebp \
    chromium \
    chromium-chromedriver \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont \
    xvfb

COPY --from=builder /app/main .

EXPOSE 8080

ENTRYPOINT ["/app/main"]