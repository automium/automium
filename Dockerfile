FROM golang:1.12.5 as builder

# Retrieve kubebuilder from Github
RUN wget --no-verbose https://github.com/kubernetes-sigs/kubebuilder/releases/download/v1.0.8/kubebuilder_1.0.8_linux_amd64.tar.gz && \
	tar xf kubebuilder_1.0.8_linux_amd64.tar.gz -C /usr/local && \
	mv /usr/local/kubebuilder_1.0.8_linux_amd64 /usr/local/kubebuilder && \
	rm kubebuilder_1.0.8_linux_amd64.tar.gz

# Copy the source into the build container
COPY . /go/src/github.com/automium/automium
WORKDIR /go/src/github.com/automium/automium

# Generate, build and test
RUN make

# Build the final minimal image
FROM alpine:latest
COPY --from=builder /go/src/github.com/automium/automium/bin/manager /bin/manager
ENTRYPOINT ["/bin/manager"]
