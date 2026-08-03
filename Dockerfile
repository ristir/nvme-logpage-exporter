FROM scratch

ARG TARGETARCH

# No USER: Docker puts --cap-add=SYS_ADMIN in the bounding set only, and it
# reaches the effective set for uid 0 alone. A non-root USER here leaves
# CapEff empty and every device read fails. --privileged is still not needed.
COPY dist/nvme_logpage_exporter-linux-${TARGETARCH} /nvme_logpage_exporter

EXPOSE 10192
ENTRYPOINT ["/nvme_logpage_exporter"]
