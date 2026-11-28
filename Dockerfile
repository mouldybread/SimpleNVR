FROM golang:1.26-alpine AS builder
WORKDIR /app
ENV GOFLAGS=-mod=readonly
COPY go.mod go.sum ./
RUN sha256sum go.mod go.sum > /tmp/dependencies.sha256 && go mod download && go mod verify && sha256sum -c /tmp/dependencies.sha256
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o simplenvr ./cmd/simplenvr && sha256sum -c /tmp/dependencies.sha256

# Runtime base pinned by digest; update deliberately and re-run Safety verification.
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
RUN apk add --no-cache ffmpeg ca-certificates && ffmpeg -hide_banner -encoders 2>/dev/null | grep -q '[[:space:]]libx264[[:space:]]'
WORKDIR /app
COPY --from=builder /app/simplenvr /app/simplenvr
COPY --from=builder /app/web/templates /app/web/templates
COPY --from=builder /app/web/static /app/web/static
EXPOSE 443
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/simplenvr"]
