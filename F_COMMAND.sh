
# MIGRATE

docker compose --profile tools run --rm migrate

docker compose -f docker-compose.dev.yml --profile tools run --rm migrate

make dev-migrate

# Copy File FROM LOCAL TO VPS
scp -r ./migrations vps1:~/absensi-api/

# Sync File FROM LOCAL TO VPS

rsync -av ./migrations/ vps1:~/absensi-api/migrations/

docker compose --profile tools run --rm migrate

# CEK VPS

curl -i --max-time 10 http://127.0.0.1:8087/api/v1/admin/health

python3 scripts/set_admin_password.py

docker compose start minio

docker compose stop minio