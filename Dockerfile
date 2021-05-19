FROM fedora

COPY validate-secrets /validate-secrets

ENTRYPOINT [ "/validate-secrets" ]
