FROM golang:1.21 AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 go build -o /app/server cmd/mobius-hotline-server/main.go && chmod a+x /app/server

FROM debian:stable-slim

# Change these as you see fit. This makes bind mounting easier so you don't have to edit bind mounted config files as root.
ARG USERNAME=mobius
ARG UID=1001
ARG GUID=1001

COPY --from=builder /app/server /app/server
COPY --from=builder /app/cmd/mobius-hotline-server/mobius/config /usr/local/var/mobius/config
RUN useradd -d /app -u ${UID} ${USERNAME}
RUN chown -R ${USERNAME}:${USERNAME} /app
EXPOSE 5500 5501

USER ${USERNAME}
ENTRYPOINT ["/app/server"]
