FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -o /out/customer-system ./cmd/customer-system

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/customer-system /customer-system
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/customer-system"]
