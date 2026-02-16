FROM golang:alpine AS builder
WORKDIR /app
# Context is project root
COPY codearena-shared-types ../codearena-shared-types
COPY codearena-realtime/go.mod codearena-realtime/go.sum ./
RUN go mod download
COPY codearena-realtime/ .
RUN go build -o main .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8081
CMD ["./main"]
