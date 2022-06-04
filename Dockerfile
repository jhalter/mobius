FROM golang:1.18 AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 go build -o /app/server/server cmd/mobius-hotline-server/main.go && chmod a+x /app/server/server

FROM scratch

WORKDIR /app/
COPY --from=builder /app/server/server ./
COPY --from=builder /app/cmd/mobius-hotline-server/mobius/config /usr/local/var/mobius/config

EXPOSE 5500 5501

CMD ["/app/server"]
