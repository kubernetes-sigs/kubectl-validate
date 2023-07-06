# build stage
FROM golang:1.20-alpine AS builder

# install kubectl-validate
RUN go install sigs.k8s.io/kubectl-validate@latest

# final stage (SIZE 98MB)
FROM scratch

# copy the binary from the builder stage
COPY --from=builder /go/bin/kubectl-validate /kubectl-validate

# set the entrypoint
ENTRYPOINT ["/kubectl-validate"]