FROM golang:alpine AS builder
WORKDIR /src
COPY . .
RUN go build -o toylb .

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root
COPY --from=builder /src/toylb .
ENTRYPOINT [ "/root/toylb" ]