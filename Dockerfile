# The Go that Kubernetes itself is built with; keep in step with .go-version.
FROM golang:1.26.6-alpine AS builder

RUN go install sigs.k8s.io/kubectl-validate@latest

FROM scratch

COPY --from=builder /go/bin/kubectl-validate /kubectl-validate

ENTRYPOINT ["/kubectl-validate"]
