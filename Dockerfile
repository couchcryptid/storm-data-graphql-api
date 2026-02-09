FROM golang:1.25-alpine AS build

RUN apk add --no-cache ca-certificates busybox-static

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

FROM gcr.io/distroless/static-debian12

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /bin/busybox.static /bin/busybox
COPY --from=build /server /server

EXPOSE 8080

ENTRYPOINT ["/server"]
