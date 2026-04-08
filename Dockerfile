FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
RUN CGO_ENABLED=0 go build -o /gecko-terminal ./cmd/thor-gecko-terminal

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /gecko-terminal /gecko-terminal

EXPOSE 1323

ENTRYPOINT ["/gecko-terminal"]
