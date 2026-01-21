FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /stub-server ./loadtest/cmd/stub-server/

FROM gcr.io/distroless/static-debian12

COPY --from=builder /stub-server /

EXPOSE 8081

CMD ["/stub-server"]
