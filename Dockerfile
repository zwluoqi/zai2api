FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/zai2api ./cmd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/zai2api-register ./cmd/register

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/zai2api /usr/local/bin/zai2api
COPY --from=builder /out/zai2api-register /usr/local/bin/zai2api-register

ENV PORT=8000
EXPOSE 8000

VOLUME ["/app/data"]
CMD ["zai2api"]
