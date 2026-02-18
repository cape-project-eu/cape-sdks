FROM golang:1.25 AS build

WORKDIR /app/src
COPY ./go.mod ./go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/bin/mockserver

FROM gcr.io/distroless/static-debian12

ARG COMMIT_SHA=local
ARG BUILD_DATE="0000-00-00T00:00:00Z"
ARG VERSION="0.0.0-dev"

LABEL org.opencontainers.image.title="CAPE API Mockserver"
LABEL org.opencontainers.image.description="A mockserver implementation that allows testing against the SecAPI implementation of CAPE."
LABEL org.opencontainers.image.authors="University of St. Gallen"
LABEL org.opencontainers.image.source="https://github.com/cape-project-eu/cape-sdk-generator/blob/${COMMIT_SHA}/mockserver/Dockerfile"
LABEL org.opencontainers.image.revision="${COMMIT_SHA}"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.base.name="gcr.io/distroless/static-debian12"

ENV GIN_MODE=release
COPY --from=build /app/bin/mockserver /
CMD ["/mockserver"]
