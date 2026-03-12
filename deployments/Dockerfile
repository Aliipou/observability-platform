FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/server /server
COPY --from=builder /src/web /web
COPY --from=builder /src/internal/storage/migrations /internal/storage/migrations
EXPOSE 9090
ENTRYPOINT ["/server"]
