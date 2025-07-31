FROM ghcr.io/kanisterio/kanister-tools:0.112.0

RUN microdnf update -y
RUN microdnf install -y findutils openssl

COPY entrypoint.sh .

RUN chmod +x ./entrypoint.sh

# ENTRYPOINT ["bash", "-c", "tail -f /dev/null"]
ENTRYPOINT ["./entrypoint.sh"]
