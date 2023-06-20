# Indicates that the windowsservercore image will be used as the base image.
FROM mcr.microsoft.com/windows/servercore:ltsc2019
# FROM golang

WORKDIR ./tests
COPY . .
RUN go get ./...
RUN go build ./cmd/ppb_worker/ppb_worker.go

EXPOSE 9085

ENTRYPOINT ["./tests/ppb_worker"]

CMD [ "echo", "Default argument from CMD instruction" ]
