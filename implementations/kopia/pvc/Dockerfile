FROM ghcr.io/kanisterio/kanister-tools:0.112.0
COPY entrypoint.sh .

RUN chmod +x ./entrypoint.sh

# ENTRYPOINT ["bash", "-c", "tail -f /dev/null"]
ENTRYPOINT ["./entrypoint.sh"]
