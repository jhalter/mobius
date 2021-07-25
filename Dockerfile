FROM golang:1.14

WORKDIR /app
COPY . .

RUN go build -o /app/server/server /app/server/server.go \
  && chmod a+x /app/server/server

EXPOSE 5500 5501 5502

WORKDIR /app/server/
CMD ["server"]