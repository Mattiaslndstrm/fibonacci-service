FROM golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /fibonacci-service .

FROM scratch
COPY --from=build /fibonacci-service /fibonacci-service
USER 65534:65534
ENTRYPOINT ["/fibonacci-service"]
