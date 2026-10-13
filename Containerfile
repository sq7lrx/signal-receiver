FROM docker.io/library/golang:1.26 AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags=-s -o /signal-receiver .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /signal-receiver /
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/signal-receiver"]
