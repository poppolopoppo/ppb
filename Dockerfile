FROM golang:1.25 AS builder

WORKDIR /app

COPY . .

RUN go mod download && GOOS=windows GOARCH=amd64 go build -o ppb_worker.exe ./cmd/ppb_worker

FROM mcr.microsoft.com/windows/servercore:ltsc2022

LABEL maintainer="ppb-build"

WORKDIR /app

COPY --chown=ContainerAdministrator:ContainerAdministrator --from=builder /app/ppb_worker.exe .

EXPOSE 9085

USER ContainerAdministrator

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD ["powershell", "-Command", "Test-Path", "C:\\app\\ppb_worker.exe"]

ENTRYPOINT ["C:\\app\\ppb_worker.exe"]