FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /meth .

FROM gcr.io/distroless/static-debian12
COPY --from=build /meth /meth
COPY config.example.yaml /config.yaml
EXPOSE 3000
USER nonroot
ENTRYPOINT ["/meth", "-config", "/config.yaml"]
