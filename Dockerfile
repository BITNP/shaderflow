FROM golang:1.23-alpine AS build

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/shaderflow ./cmd/shaderflow

FROM alpine:3.21
WORKDIR /app
COPY --from=build /out/shaderflow ./shaderflow
COPY web ./web

EXPOSE 8080
ENTRYPOINT ["./shaderflow", "-config", "/conf/server.docker.dev.json"]
