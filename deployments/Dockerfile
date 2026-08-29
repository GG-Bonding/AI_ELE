FROM golang:1.25-bookworm AS build

WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/aee-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/aee-server /app/aee-server
COPY configs/config.example.yaml /app/configs/config.yaml

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/aee-server", "-config", "/app/configs/config.yaml"]
