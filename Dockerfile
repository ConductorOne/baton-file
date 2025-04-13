FROM gcr.io/distroless/static-debian11:nonroot
ENTRYPOINT ["/baton-file"]
COPY baton-file /