FROM scratch

# No USER: Docker puts --cap-add=SYS_ADMIN in the bounding set only, and it
# reaches the effective set for uid 0 alone. A non-root USER here leaves
# CapEff empty and every device read fails. --privileged is still not needed.
COPY nvme_logpage_exporter /nvme_logpage_exporter

EXPOSE 9683
ENTRYPOINT ["/nvme_logpage_exporter"]
