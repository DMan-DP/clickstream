# Migrations image for the Kubernetes Job (deploy/k8s/migrate.yaml): the
# goose image from compose with the SQL files baked in, so the cluster
# needs nothing from the host. Build context is migrations/:
#   docker compose build migrate
# A new migration means a new image: build, minikube image load, then
# delete and apply the Job again (see the comment in migrate.yaml).
FROM ghcr.io/kukymbr/goose-docker:3.26.0

# The entrypoint runs goose with -dir=/migrations
COPY *.sql /migrations/
