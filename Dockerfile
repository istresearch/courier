FROM golang:latest

RUN set -ex; \
    addgroup --system courier; \
    adduser --system --ingroup courier courier

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN go build -o main ./cmd/courier/main.go

EXPOSE 8080

USER courier

ENTRYPOINT []
CMD ["courier"]