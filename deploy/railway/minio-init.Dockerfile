FROM minio/mc:RELEASE.2025-08-13T08-35-41Z

COPY deploy/compose/prod/minio-app-policy.json /config/minio-app-policy.json
COPY deploy/railway/minio-init.sh /usr/local/bin/gamegen-minio-init

ENTRYPOINT ["/bin/sh", "/usr/local/bin/gamegen-minio-init"]
