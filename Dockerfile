FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/zai2api ./cmd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/zai2api-register ./cmd/register

FROM alpine:3.20

# chromium 供 captcha 自动生成（CAPTCHA_AUTO_GEN=true）时的无头浏览器使用
RUN apk add --no-cache ca-certificates tzdata \
    chromium nss freetype harfbuzz ttf-freefont

WORKDIR /app
COPY --from=builder /out/zai2api /usr/local/bin/zai2api
COPY --from=builder /out/zai2api-register /usr/local/bin/zai2api-register

ENV PORT=8000
# 供 captcha 生成器定位浏览器（启用 CAPTCHA_AUTO_GEN 时生效）
ENV CAPTCHA_BROWSER_BIN=/usr/bin/chromium-browser
EXPOSE 8000

VOLUME ["/app/data"]
CMD ["zai2api"]
