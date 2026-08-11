# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o simplenvr .

# Final runtime stage
FROM alpine:latest
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=builder /app/simplenvr /app/simplenvr
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/static /app/static

EXPOSE 443
ENTRYPOINT ["/app/simplenvr"]
