FROM --platform=linux/amd64 redhat/ubi8-minimal

COPY entrypoint.sh /
COPY simi /

ENTRYPOINT [ "./entrypoint.sh" ]
