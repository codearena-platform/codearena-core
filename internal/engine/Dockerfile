FROM golang:alpine AS builder
WORKDIR /app
# Context is project root
COPY codearena-shared-types ../codearena-shared-types
COPY codearena-simulation/go.mod codearena-simulation/go.sum ./
RUN go mod download
COPY codearena-simulation/ .
RUN go build -o main ./main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 50051 50052
CMD ["./main"]
