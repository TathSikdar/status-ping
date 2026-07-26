# Multi-stage build for the Go backend.
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY endpoints.json ./
# No go.sum committed; tidy resolves + verifies deps at build time.
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o /statusping ./cmd/statusping

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /statusping /app/statusping
COPY endpoints.json /app/endpoints.json
EXPOSE 8080
CMD ["/app/statusping"]
