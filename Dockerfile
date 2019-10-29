FROM golang:latest as builder

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o courier ./cmd/courier/main.go



FROM alpine:3.7

ENV USER=courier
ENV UID=13337
ENV GID=13337

RUN addgroup -g "$GID" "$USER" \
    && adduser \
    -D \
    -g "" \
    -h "$(pwd)" \
    -G "$USER" \
    -H \
    -u "$UID" \
    "$USER"

RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

WORKDIR /app

COPY --from=builder /app/courier .

EXPOSE 8080

USER courier

ENTRYPOINT []
CMD ["courier"]