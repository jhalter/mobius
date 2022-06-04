FROM golang:1.18

WORKDIR /app
COPY . .

RUN go build -o /app/server/server cmd/mobius-hotline-server/main.go \
  && chmod a+x /app/server/server

EXPOSE 5500 5501

WORKDIR /app/server/
CMD ["./server"]

