FROM golang:1.18 AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 go build -o /app/server cmd/mobius-hotline-server/main.go && chmod a+x /app/server

FROM scratch

COPY --from=builder /app/server /app/server
COPY --from=builder /app/cmd/mobius-hotline-server/mobius/config /usr/local/var/mobius/config

EXPOSE 5500 5501

ENTRYPOINT ["/app/server"]
CMD ["/app/server"]
