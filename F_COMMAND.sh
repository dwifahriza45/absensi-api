
# MIGRATE

docker compose --profile tools run --rm migrate

docker compose -f docker-compose.dev.yml --profile tools run --rm migrate

make dev-migrate