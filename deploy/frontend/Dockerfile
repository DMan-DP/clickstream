# Frontend image for Kubernetes (deploy/k8s/frontend.yaml): nginx with the
# page and the config template baked in, so the cluster needs nothing from
# the host. Build context is the repository root, the files it may see are
# listed in Dockerfile.dockerignore next to this file (the root
# .dockerignore lets only the Go sources through):
#   docker compose build frontend
# An edit of the page means a new image: build, minikube image load, then
# kubectl -n clickstream rollout restart deployment frontend.
FROM nginx:1.28.0-alpine

# Upstream for /events, rendered into the template at start. The Service
# name works both in compose and in the cluster; override it with the
# environment variable of the container.
ENV TRACKER_API_URL=http://tracker-api:8080

COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY frontend/ /usr/share/nginx/html/
