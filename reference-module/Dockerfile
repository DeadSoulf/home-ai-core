FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/home-ai-reference-module .

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/DeadSoulf/home-ai-core"
LABEL org.opencontainers.image.title="Home-AI Reference Module"
LABEL org.opencontainers.image.description="Reference implementation for Home-AI Docker Module Runtime v2"
COPY --from=build /out/home-ai-reference-module /home-ai-reference-module
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/home-ai-reference-module"]
