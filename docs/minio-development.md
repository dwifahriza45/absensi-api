# MinIO Docker tanpa Cloudflare/account billing

Compose utama (`docker-compose.yml`) dan development (`docker-compose.dev.yml`) sama-sama menyediakan MinIO melalui adapter S3 yang sama dengan R2. Tidak memerlukan kartu kredit. Foto baru masuk ke bucket private; API admin tetap menyajikannya melalui cookie/guard, sehingga FE, unduh foto, dan PDF tidak perlu berubah. Keduanya memakai project `absensi-dev` dan volume MinIO/DB yang sama, bukan membuat storage atau DB kedua.

## Menjalankan

```sh
make dev-setup
make dev-migrate
make dev
```

`dev-setup` menjalankan `scripts/setup_minio.py`: menyiapkan `.env`, membuat credentials acak root MinIO dan aplikasi yang berbeda, serta memasangkan `S3_*` dengan akun aplikasi MinIO. Pada `.env` yang sudah ada, DB_PASSWORD, JWT_SECRET, dan PHONE_HASH_SECRET tidak diubah. Pada file baru, JWT/phone secrets ikut dibuat acak. Script menolak `APP_ENV=production`; HTTP MinIO ini untuk development.

Menjalankan setup ulang mempertahankan credentials MinIO yang sudah ada, tetapi memilih MinIO sebagai storage aktif. Jangan menjalankannya pada konfigurasi R2 yang ingin tetap dipakai. Tidak ada secret yang dicetak ke terminal.

Alamat default (mengikuti `.env`):

| Layanan | Alamat |
|---|---|
| API absensi | http://127.0.0.1:8082 |
| MinIO S3 API | http://127.0.0.1:9000 |
| MinIO Console | http://127.0.0.1:9001 |

Login Console menggunakan `MINIO_ROOT_USER` dan `MINIO_ROOT_PASSWORD` dari `.env`. Akun aplikasi memakai `MINIO_APP_ACCESS_KEY`/`MINIO_APP_SECRET_KEY`, tidak diberi akses admin, dan hanya mendapat list/region bucket ini serta Get/Put/Delete pada prefix yang diatur. Jangan menaruh credentials pada env/frontend Vite.

`minio-init` membuat bucket `absensi-development`, mematikan anonymous access, dan membuat policy akun aplikasi. API menunggu init berhasil sebelum mulai. Init aman dijalankan ulang dengan settings yang sama. Port MinIO hanya bind ke loopback, tidak dipublikasikan ke Wi-Fi/internet.

Untuk Compose utama, gunakan `docker compose up -d --build` setelah konfigurasi production (secrets, password admin, serta HTTPS frontend/API) sudah siap. Mode utama tetap `APP_ENV=production` dan Secure cookies; mode dev tetap Air. `.env` MinIO memasang `S3_ALLOW_MINIO_HTTP=true`, pengecualian yang hanya menerima endpoint persis `http://minio:9000` di jaringan internal Compose. HTTP endpoint luar tetap ditolak di production, sekalipun flag ini true. Untuk R2 gunakan HTTPS dan set flag false sesuai template R2. Jangan membuka port MinIO langsung ke internet.

`.env` memakai `S3_ENDPOINT=http://minio:9000` untuk API di Docker. Jika API Go berjalan langsung di host, override endpoint menjadi `http://127.0.0.1:9000`, misalnya `make run S3_ENDPOINT=http://127.0.0.1:9000`. FE tetap memanggil API; HP tidak perlu mengakses MinIO langsung. Kamera/GPS HP tetap memerlukan HTTPS pada frontend.

## Data dan volume

Data PostgreSQL tetap di `absensi-dev_pgdata`; foto MinIO memakai volume baru `absensi-dev_minio-data`. Volume foto local lama `absensi-dev_storage` tetap dipertahankan untuk migrasi/restore. Tidak ada DB kedua atau volume lama yang dihapus.

Jika sebelumnya sudah ada foto local, jangan langsung membuka API S3 sebelum menyalinnya:

```sh
docker compose -f docker-compose.dev.yml build app minio minio-init
docker compose -f docker-compose.dev.yml up -d db minio minio-init
docker compose -f docker-compose.dev.yml stop app
# Dry run terlebih dahulu:
docker compose -f docker-compose.dev.yml run --rm --no-deps --entrypoint photoctl app \
  migrate-local --local-dir /app/storage
# Salin object yang belum ada, verifikasi SHA-256; tidak menimpa/menghapus sumber:
docker compose -f docker-compose.dev.yml run --rm --no-deps --entrypoint photoctl app \
  migrate-local --local-dir /app/storage --apply
# Verifikasi ulang: pending harus 0.
docker compose -f docker-compose.dev.yml run --rm --no-deps --entrypoint photoctl app \
  migrate-local --local-dir /app/storage
docker compose -f docker-compose.dev.yml up -d --no-deps app
```

Jangan `down -v`. `make dev-down` menghentikan container tanpa menghapus volume. Menghapus volume MinIO menghapus object meskipun metadata database masih ada. Backup sumber sebelum perubahan deployment; jangan menghapus foto lama sebelum verifikasi dan salinan di luar mesin tersedia.

## Sumber image

Image `absensi-minio:local` dan `absensi-minio-mc:local` dibuild oleh Compose, bukan diambil dari image pihak ketiga. Dockerfile memakai source resmi server **RELEASE.2025-10-15T17-29-55Z** (rilis perbaikan keamanan), serta mc **RELEASE.2025-08-13T08-35-41Z**, dengan compiler Go 1.27.3. Build pertama mengunduh banyak dependency; build berikutnya memakai cache.

[Rilis resmi server](https://github.com/minio/minio/releases/tag/RELEASE.2025-10-15T17-29-55Z) menyarankan build container dari source. Repository community upstream telah diarsipkan; setup ini untuk development, bukan janji maintenance/security update production. Production tetap wajib HTTPS dan pengaturan storage/deployment yang sesuai.

## Backup dan pindah ke R2

Karena driver tetap `s3`, [backup incremental](backup-otomatis.md) otomatis membaca foto dari MinIO memakai credentials aplikasi. Bucket private dan S3 prefix tetap dihormati. Tool `backup.py backup` lama khusus local filesystem; gunakan `backup_incremental.py nightly` dan `materialize` untuk MinIO.

MinIO pada disk yang sama **tetap menggunakan kapasitas disk mesin tersebut**. Ini bukan tambahan kapasitas untuk VPS 60 GB dan bukan backup di luar server. Repository backup juga menambah salinan; tarik ke laptop/disk lain seperti konfigurasi backup sebelumnya.

Nanti R2 menggunakan [template Cloudflare](../ops/cloudflare-r2.env.example) dan adapter yang sama. Mengganti endpoint saja tidak memindahkan object MinIO. Ambil snapshot MinIO yang terverifikasi, `materialize` ke file asli, lalu gunakan `photoctl migrate-local --local-dir <staging> --apply` ke R2 seperti [panduan migrasi](cloudflare-r2.md). Hentikan app selama snapshot/migrasi cutover agar tidak ada foto baru yang tertinggal. Jangan hapus bucket MinIO sebelum seluruh data tujuan dan backup lolos verifikasi.

## Verifikasi storage aktif

```sh
docker compose -f docker-compose.dev.yml ps -a
docker compose -f docker-compose.dev.yml logs minio-init
# Tes opt-in membuat dan menghapus hanya object sementara, tanpa attendance/employee:
docker compose -f docker-compose.dev.yml exec -T -e TEST_S3_LIVE=1 app \
  go test ./internal/pkg/storage -run '^TestS3LiveConfiguredStorage$' -count=1 -timeout=90s -v
python3 -m unittest discover -s scripts -p 'test_setup_minio.py' -v
```

Tes live memeriksa upload/read/delete, conditional create tanpa overwrite, anonymous access ditolak, dan penulisan di luar prefix ditolak. Credentials tidak ditampilkan dalam output.
