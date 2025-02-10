FROM --platform=linux/amd64 redhat/ubi8-minimal
RUN microdnf install tar # required to run kubectl cp
COPY bench-consumer /
ENTRYPOINT [ "/bench-consumer" ]
