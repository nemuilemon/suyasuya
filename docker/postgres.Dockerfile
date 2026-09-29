FROM postgres:16-alpine
# Same schema and role initialization as Compose; executed only on an empty PGDATA.
COPY schema.sql /docker-entrypoint-initdb.d/10-schema.sql
COPY --chmod=0555 docker/20-roles.sh /docker-entrypoint-initdb.d/20-roles.sh
