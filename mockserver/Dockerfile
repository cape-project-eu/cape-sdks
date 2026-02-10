FROM golang:1.25 AS build

WORKDIR /app/src
COPY ./go.mod ./go.sum ./
RUN go mod download

COPY ./mockserver ./mockserver
COPY ./provider ./provider
WORKDIR /app/src/mockserver
RUN CGO_ENABLED=0 go build -o /app/bin/mockserver

FROM gcr.io/distroless/static-debian12

ENV GIN_MODE=release
COPY --from=build /app/bin/mockserver /
CMD ["/mockserver"]
