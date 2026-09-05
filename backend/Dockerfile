FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM alpine:3.20 AS api
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["api"]

FROM alpine:3.20 AS worker
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/worker /usr/local/bin/worker
ENTRYPOINT ["worker"]
