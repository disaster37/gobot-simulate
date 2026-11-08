# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
ARG VERSION=0.1.0-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/.output/public ./webui/dist
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/disaster37/gobot-simulate.Version=${VERSION}" \
    -o /out/gobot-simulate ./cmd/gobot-simulate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gobot-simulate /gobot-simulate
EXPOSE 8080
ENTRYPOINT ["/gobot-simulate", "--addr", "0.0.0.0", "--port", "8080"]
