# Deploy API di VPS

Gunakan `docker-compose.vps.yml` untuk deployment baru. Compose lokal/dev tidak berubah. Project VPS memakai volume `absensi-production_pgdata` dan `absensi-production_minio-data`. Jangan gunakan `down -v` untuk update. Jangan mengganti project Compose deployment lama tanpa memindahkan datanya terlebih dahulu.

## Image Docker Hub

Buat tiga repository: `absensi-api`, `absensi-minio`, `absensi-minio-mc`. Pada repository GitHub API (isi project ini berada di root), tambahkan secrets `DOCKERHUB_USERNAME` dan `DOCKERHUB_TOKEN` dengan izin push. Push ke main/master atau jalankan workflow manual. Ketiga image memakai tag `sha-<40 karakter commit>` yang sama; tunggu semua job berhasil.

## Persiapan sekali di VPS

Jalankan dari folder project API. Isi username dan SHA sesuai hasil Actions:

```bash
python3 scripts/setup_vps.py --dockerhub-user USERNAME --tag sha-COMMIT_SHA_LENGKAP
docker network inspect absen >/dev/null 2>&1 || docker network create absen
docker compose --env-file .env -f docker-compose.vps.yml config --quiet
docker compose --env-file .env -f docker-compose.vps.yml pull
docker compose --env-file .env -f docker-compose.vps.yml up -d db minio
docker compose --env-file .env -f docker-compose.vps.yml --profile tools run --rm migrate
python3 scripts/set_admin_password.py
docker compose --env-file .env -f docker-compose.vps.yml up -d app
docker compose --env-file .env -f docker-compose.vps.yml ps -a
curl --fail http://127.0.0.1:8087/api/v1/admin/health
```

Setup menghasilkan `.env` privat dengan secret acak dan akses aplikasi MinIO terpisah dari root. Tidak menimpa konfigurasi yang sudah ada. Origin default HTTPS alotive.com dan www.alotive.com; sesuaikan jika domain berubah. Jangan mengganti PHONE_HASH_SECRET setelah karyawan dibuat karena dipakai untuk data nomor HP. Simpan seluruh `.env` bersama backup secara aman.

Password admin diminta secara interaktif oleh script, tidak masuk shell history. Login email awal `admin@absensi.local`. API production menolak password seed bawaan, sehingga langkah ganti password wajib. MinIO init otomatis saat app dimulai; status Exited (0) pada service init normal.

API hanya membuka `127.0.0.1:8087` dan punya alias `absensi-api:8087` di network `absen`. Database tidak membuka port VPS. MinIO 9000/9001 hanya loopback; akses console dengan SSH tunnel bila perlu. Bucket private dan foto tetap lewat endpoint API berautentikasi. MinIO berada di disk VPS, jadi tetap membutuhkan backup ke perangkat lain.

## Nginx domain

Dalam server HTTPS yang sudah melayani frontend, ubah blok API menjadi:

```nginx
location /api/v1/ {
    proxy_pass http://127.0.0.1:8087;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 10m;
    proxy_connect_timeout 5s;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    proxy_buffering off;
}
```

```bash
sudo nginx -t && sudo systemctl reload nginx
curl --fail https://alotive.com/api/v1/admin/health
```

Frontend admin/pegawai memakai API_UPSTREAM=http://absensi-api:8087 dan API_NETWORK=absen. Login melalui domain HTTPS agar cookie Secure terkirim.

## Update

Backup database, foto, dan `.env` terlebih dahulu (lihat panduan backup). Ubah IMAGE_TAG, MINIO_IMAGE, MINIO_MC_IMAGE ke SHA yang telah berhasil dipublish, tanpa mengubah secret/bucket/prefix. Pull image, jalankan migration, lalu up:

```bash
docker compose --env-file .env -f docker-compose.vps.yml pull
docker compose --env-file .env -f docker-compose.vps.yml --profile tools run --rm migrate
docker compose --env-file .env -f docker-compose.vps.yml up -d
```

Untuk backup otomatis, gunakan file Compose `docker-compose.vps.yml`, env file `.env`, dan nama project `absensi-production` pada konfigurasi backup. Jadwal backup belum diaktifkan otomatis.

Konfigurasi VPS S3 tidak memakai `STORAGE_DIR` atau volume foto lokal. Backup foto mengambil objek dari MinIO melalui API S3. Dukungan storage lokal pada kode tetap tersedia untuk alat migrasi/restore. Jika `.env` dibuat sebelumnya, baris `STORAGE_DIR` boleh dihapus; jangan menghapus volume lama sebelum memastikan backup dan migrasi selesai.
