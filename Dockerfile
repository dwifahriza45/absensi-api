FROM golang:1.27.3-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /server ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -o /photoctl ./cmd/photoctl

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=builder /server ./server
COPY --from=builder /photoctl /usr/local/bin/photoctl
RUN mkdir -m 700 storage && chown app:app storage
USER app
EXPOSE 8080
CMD ["./server"]
