FROM golang:1.16 as builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -mod=readonly -v -a exporter.go

FROM scratch
COPY --from=builder /app/exporter /
EXPOSE 8080
ENTRYPOINT ["/exporter"]
