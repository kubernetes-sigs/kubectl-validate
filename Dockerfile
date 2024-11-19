FROM golang:1.23-alpine AS builder

RUN go install sigs.k8s.io/kubectl-validate@latest

FROM scratch

COPY --from=builder /go/bin/kubectl-validate /kubectl-validate

ENTRYPOINT ["/kubectl-validate"]
