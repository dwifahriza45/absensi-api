# absensi-api

Compose utama dan development sekarang sama-sama menyediakan **MinIO private** tanpa akun Cloudflare/kartu kredit, memakai volume yang sama. Untuk dev jalankan `make dev-setup`, `make dev-migrate`, lalu `make dev`. Console MinIO: http://127.0.0.1:9001; credentials tersimpan di `.env`. Panduan kedua mode, foto lama, volume, dan pindah ke R2: [MinIO Docker](docs/minio-development.md).

Cloudflare R2 private tersedia melalui adapter S3-compatible (AWS SDK Go v2). Konfigurasi, migrasi foto lama, serta backup R2: [panduan Cloudflare R2](docs/cloudflare-r2.md). Development tetap dapat memakai `STORAGE_DRIVER=local`; aktifkan `s3` setelah bucket dan credentials disiapkan. Foto tetap diakses melalui API cookie admin, bukan URL publik.

Backup malam incremental dan penarikan otomatis ke laptop: [panduan konfigurasi server + laptop](docs/backup-otomatis.md). Template systemd 02.00 WIB dan launchd Mac tersedia di `ops/backup/`, belum diaktifkan. Database penuh per snapshot, foto baru saja, checksum, serta export bulanan dan bundle restore offline tersedia melalui `scripts/backup_incremental.py`.

Backup database, storage asli, dan arsip foto per bulan: lihat [panduan backup/restore](docs/backup-restore.md). Tool operator: `python3 scripts/backup.py backup --month YYYY-MM --output /lokasi/folder-baru --compose-file docker-compose.yml`. Perintah menghentikan app sementara dan menyalakannya kembali setelah snapshot selesai; tidak menghapus data sumber.

Backend absensi Go + PostgreSQL. Mengikuti pola coffee-api: `cmd/api`, wiring dan middleware di `internal/app`, domain dengan handler/service/repository, konfigurasi/database di `internal/pkg`, dan migration SQL berurutan.

Route dikelompokkan melalui helper di `internal/app/routes.go` di atas `http.ServeMux`: prefix `/api/v1/admin` dan `/api/v1/karyawan`, lalu subgroup per fitur (`/auth`, `/employees`, `/attendances`, dan lainnya). Middleware role dipasang pada group yang dilindungi; login, refresh, dan logout tidak mewajibkan access token, sementara `/auth/me` memakai guard sesuai role.

## Jalankan dengan Docker

```sh
cp .env.example .env
# Isi JWT_SECRET dan PHONE_HASH_SECRET dengan nilai acak >=32 karakter.
# Untuk production, isi endpoint storage HTTPS sesuai docs/cloudflare-r2.md.
# .env.example sekarang memakai MinIO HTTP untuk Compose development, bukan production.
# Untuk development dengan akun seed: gunakan Compose dev (lihat bagian Air).
# Production menolak password admin bawaan; lihat docs/security.md.
docker compose up -d db
docker compose run --rm migrate
# Ganti password admin seed sebelum menjalankan app production.
docker compose up -d --build app
```

API: `http://localhost:8082`. Database lokal: `localhost:5434`. Port ini dipisah dari coffee-api; bisa diubah lewat `HTTP_PORT` dan `DB_PORT`. Service `migrate` memakai profile `tools`, mengikuti coffee-api. Jalankan migration secara terpisah sebelum menyalakan API; app menunggu database sehat. Menjalankan `docker compose up` saja tidak menjalankan migration. Data PostgreSQL dan foto disimpan dalam volume persisten.

Migration `000002` menambahkan akun awal dengan role **admin**:

- Email: `admin@absensi.local`
- Password: `Admin123!`

Ganti password akun awal sebelum deployment, misalnya melalui SQL dengan hash bcrypt dari `crypt('password-baru', gen_salt('bf', 10))`. Aktifkan `COOKIE_SECURE=true` saat menggunakan HTTPS.

```sh
docker compose logs -f app
# Terapkan migration berikutnya saat stack sudah berjalan:
docker compose run --rm migrate
```

Lokal tanpa container API: Go 1.27.3+, `cp .env.example .env`, `make db-up`, `make migrate-up`, `make run`. Makefile membaca `.env`; binary membaca environment. Jalankan `make test` dan `make vet` untuk pemeriksaan. Uji integrasi sesi database: `TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/pkg/token -v` (membuat dan menghapus schema uji sendiri).

## Login dan session

Admin login menggunakan email/password bcrypt seperti coffee-api. Karyawan login menggunakan **nama lengkap dan NIK KTP 16 digit terdaftar** dari data pegawai aktif. NIK dicari lewat indeks unik. Kontak disimpan terenkripsi AES-GCM dan hanya dibuka pada API CRUD/list/detail admin. Nama dicocokkan persis tanpa membedakan huruf besar/kecil setelah spasi dirapikan. Nama yang sama tetap bisa login karena NIK membedakan identitas pegawai.

Login mengirim cookie `access_token` dan `refresh_token` (`HttpOnly`, `SameSite=Lax`). Path cookie dipisah `/api/v1/admin` dan `/api/v1/karyawan`, sehingga kedua sesi bisa dipakai pada browser yang sama. Access token JWT berlaku 15 menit. Refresh token acak disimpan sebagai SHA-256 di tabel `sessions`, berlaku 7 hari untuk admin dan 90 hari untuk karyawan. Refresh merotasi token sekali pakai; logout mencabut sesi sehingga access token lama juga tidak berlaku. Akun nonaktif tidak dapat memakai sesi maupun refresh.

Frontend harus mengirim `credentials: 'include'` dan memakai origin yang ada di `CORS_ORIGINS`. Gunakan frontend/API pada site yang sama untuk cookie SameSite Lax. Token tidak lagi dikirim dalam body JSON; frontend menggunakan cookie HttpOnly dan tidak menyimpan token di localStorage. Endpoint refresh/logout menggunakan cookie refresh. Operasi logout dibatasi 10 detik; timeout menghasilkan HTTP 504 (NOK), bukan respons sukses. Semua perubahan dari browser diperiksa terhadap daftar origin CORS.

```sh
curl -c admin.cookies -H 'Content-Type: application/json' \
  -d '{"email":"admin@absensi.local","password":"Admin123!"}' \
  http://localhost:8082/api/v1/admin/auth/login
curl -b admin.cookies http://localhost:8082/api/v1/admin/auth/me
curl -b admin.cookies -c admin.cookies -X POST http://localhost:8082/api/v1/admin/auth/refresh
curl -b admin.cookies -X POST http://localhost:8082/api/v1/admin/auth/logout

curl -c karyawan.cookies -H 'Content-Type: application/json' \
  -d '{"fullname":"Nama Lengkap Karyawan","nik":"0123456789012345"}' \
  http://localhost:8082/api/v1/karyawan/auth/login
```

## API

Response JSON mengikuti `coffee-api`: `code` numerik sesuai HTTP status, `status` **OK/NOK**, `message`, `error` boolean, serta `data`/`total`/`valid` bila tersedia. Error tidak lagi berupa objek nested; detail validasi berada di `valid` dengan key yang sama seperti field request. Lihat [kontrak request/response](docs/api-response.md).

Field kosong atau hanya spasi menghasilkan HTTP 400 dan seluruh pesan field sekaligus. Email/NIK juga diperiksa formatnya sebelum autentikasi. Password tidak di-trim dan tidak pernah dikembalikan dalam error. Login memakai batas body 4 KiB; JSON lainnya tetap 1 MiB, import 4 MiB. Route lama tanpa prefix role sudah dipindah; frontend perlu memperbarui URL.

| Method | Path | Akses / body |
|---|---|---|
| GET | `/api/v1/admin/health` | Publik |
| POST | `/api/v1/admin/auth/login` | Publik, `{email,password}` |
| POST | `/api/v1/admin/auth/refresh` | Cookie refresh admin |
| POST | `/api/v1/admin/auth/logout` | Cookie refresh admin |
| GET | `/api/v1/admin/auth/me` | Admin |
| GET | `/api/v1/admin/employees` | Admin; `mitra,bidang,bidang_id,q` |
| GET | `/api/v1/admin/employees/filter-options` | Admin |
| POST | `/api/v1/admin/employees/import` | Admin; `{rows:[{name,nik,mitra,bidang,phone,employee_id?}],deactivateMissing}` |
| GET | `/api/v1/admin/attendances` | Admin; `from,to,mitra,bidang,q` |
| GET | `/api/v1/admin/attendances/version` | Admin; `date` |
| GET | `/api/v1/admin/photos/{id}` | Admin |
| GET | `/api/v1/karyawan/employees/search` | Publik; `q` minimal 3 huruf, maksimal 4 hasil |
| POST | `/api/v1/karyawan/auth/login` | Publik; `{fullname,nik}` |
| POST | `/api/v1/karyawan/auth/refresh` | Cookie refresh karyawan |
| POST | `/api/v1/karyawan/auth/logout` | Cookie refresh karyawan |
| GET | `/api/v1/karyawan/auth/me` | Karyawan |
| GET | `/api/v1/karyawan/attendances/today` | Karyawan |
| POST | `/api/v1/karyawan/attendances/check-in` | Karyawan; multipart `selfie,lat,lng,accuracy` |
| POST | `/api/v1/karyawan/attendances/check-out` | Karyawan; multipart di atas; alasan diambil dari permintaan pulang awal |
| POST | `/api/v1/karyawan/attendances/early-leave` | Karyawan; `{reason}` |
| POST | `/api/v1/karyawan/work-reports` | Karyawan; multipart `session_no,photo` |

Karyawan hanya mengakses profil dan aktivitas absensinya sendiri, termasuk foto hasil kerja yang sudah menjadi bagian alur absen. Data pegawai, rekap, import dan foto admin dilindungi role admin.

## Aturan yang dipertahankan

Waktu absensi memakai waktu server `APP_TIMEZONE`. Jadwal kerja dan jumlah sesi foto diatur lewat `SCHEDULE_*`. Foto sesi yang dikirim ulang tidak membuat data ganda. Selfie dan lokasi wajib; akurasi di atas 100 m ditandai perlu ditinjau. Pulang tidak dibatasi jam selesai, tetapi seluruh sesi laporan harus lengkap kecuali sudah mengajukan pulang lebih awal.

NIK menjadi identitas unik import dan kredensial login. Nomor HP tetap di-hash dengan HMAC untuk pencocokan kontak, termasuk melengkapi NIK data lama. Jangan mengganti `PHONE_HASH_SECRET` setelah import; secret ini juga menurunkan kunci enkripsi kontak dan harus disimpan dalam backup yang aman. Nomor lama yang hanya mempunyai hash perlu diisi ulang/import untuk bisa dibaca admin. Limiter login admin sepuluh dan karyawan lima percobaan per akun/IP selama 15 menit ditambah limit karyawan 3000 request per IP per menit dan admin 100 request per IP per menit disimpan di memori, sehingga deployment beberapa replika membutuhkan limiter bersama. Jadwal masih sama untuk semua penempatan.

## ID bisnis dan relasi

Semua tabel domain mempertahankan UUID `id` sebagai primary key. Setelah migration `000006_business_key_references`, foreign key dan lookup aplikasi memakai kolom ID bisnis berikut:

| Tabel | Kolom | Contoh |
|---|---|---|
| `employees` | `employee_id` | `EMP-20261031-000001` |
| `users` (admin) | `user_id` | `USER-20261031-000001` |
| `mitra` | `mitra_id` | `MTR-20261031-000001` |
| `attendances` | `attendance_id` | `ATT-20261031-000001` |
| `photos` | `photo_id` | `PHO-20261031-000001` |
| `work_reports` | `work_report_id` | `WRP-20261031-000001` |
| `sessions` | `session_id` | `SES-20261031-000001` |

| Referensi | Mengarah ke |
|---|---|
| `employees.mitra_id` | `mitra.mitra_id` (`INT`) |
| `attendances.employee_id` | `employees.employee_id` (`EMP`) |
| `attendances.check_in_selfie_id` / `check_out_selfie_id` | `photos.photo_id` (`PHO`) |
| `work_reports.attendance_id` | `attendances.attendance_id` (`ATT`) |
| `work_reports.photo_id` | `photos.photo_id` (`PHO`) |
| `sessions.user_id` | `users.user_id` (`USER`, admin) |
| `sessions.employee_id` | `employees.employee_id` (`EMP`, karyawan) |

Sesi memiliki tepat satu pemilik sesuai role. FK pemilik sesi memakai `ON DELETE CASCADE`; hapus akun berarti mencabut sesinya. Mitra yang dipakai tidak dapat dihapus. Kode bisnis yang masih direferensikan tidak dapat diubah. Import tetap menerima nama `mitra`, membuat/memakai master berdasarkan nama, dan menyimpan kode `INT`; mitra kosong menjadi NULL.

Nomor dihasilkan otomatis oleh trigger, naik per prefix/tanggal Asia/Jakarta dengan counter atomik. Counter `EMP` dan `USER` sekarang terpisah. Upsert/import ulang mempertahankan kode; nomor yang telah terhapus tidak dipakai kembali. Konflik insert bersamaan dapat meninggalkan celah nomor. Batas setiap prefix/tanggal adalah 999999.

Migration 000006 memetakan referensi data lama dari UUID ke kode bisnis. Kode karyawan lama berubah dari `USER` menjadi `EMP` dengan tanggal/sufiks yang sama; counter EMP disetel melanjutkan angka terbesar. UUID, data absensi, metadata/file foto tetap dipertahankan. Sesi lama tanpa akun pemilik dicabut.

Response pegawai memakai `employee_id` dan `mitra_id`; `user_id` karyawan dan alias UUID `mitraId` dihapus. Response absensi memakai `attendance_id`, `employee_id`, `check_in_selfie_id`, dan `check_out_selfie_id`. Foto sesi admin memakai `photo_id`. Endpoint foto admin menerima kode PHO dan mengirim `X-Photo-ID`; UUID bukan lagi lookup key. UUID `id` tetap disertakan pada response utama sebagai metadata, termasuk ID UUID pegawai pada rekap admin.

JWT memakai `session_id` SES sebagai jti dan kode USER/EMP sebagai subject. Setelah upgrade, access token lama yang memakai UUID perlu di-refresh atau login ulang. Refresh token valid yang tersimpan tetap bisa dipakai untuk menerbitkan access token baru.

### Upgrade ke migration 000006

Hentikan app versi lama sebelum mengubah skema, lalu jalankan migration dan app baru. Ini bukan migration untuk rolling deployment tanpa downtime.

```sh
# Development
docker compose -f docker-compose.dev.yml stop app
make dev-migrate
docker compose -f docker-compose.dev.yml up -d --build app

# Production
docker compose stop app
docker compose run --rm migrate
docker compose up -d --build app
```

Tes upgrade, rollback, FK, dan ID paralel: `TEST_DATABASE_URL="$DATABASE_URL" go test -race ./...` pada database uji.

## Development dengan Air

```sh
make dev-setup # menyiapkan .env privat + credentials MinIO; secret app existing dipertahankan
make dev-migrate
make dev
# atau:
docker compose -f docker-compose.dev.yml run --rm migrate
docker compose -f docker-compose.dev.yml up --build
```

API dev tersedia di `http://localhost:8082`, PostgreSQL di `localhost:5434`. Compose dev dan utama sama-sama memakai project `absensi-dev`, sehingga memakai satu service DB (`absensi-dev-db-1`) dan volume PostgreSQL/foto yang sama. Pilih satu mode app pada satu waktu; menjalankan file lainnya akan mengubah konfigurasi service app yang sama. Nama project ini dipertahankan agar volume yang sudah ada tetap terbaca. Override `-p` atau `COMPOSE_PROJECT_NAME` akan membuat project terpisah, jadi gunakan hanya bila memang ingin stack uji terisolasi.

Source dipasang langsung ke `/app`. Perubahan `.go`, `go.mod`, atau `go.sum` memicu build dan restart lewat Air. Polling diaktifkan agar perubahan bind mount terbaca di Docker Desktop. Folder `tmp`, `storage`, dan `vendor` tidak dipantau; cache module, cache build, binary sementara, dan storage memakai volume terpisah. Air dipasang pada versi tetap `v1.63.0`.

Migration dijalankan terpisah lewat service `migrate` dalam profile `tools`. Jalankan `make dev-migrate` sebelum startup pertama. Untuk migration baru ketika Air sedang berjalan, gunakan perintah yang sama; perubahan skema yang tidak kompatibel membutuhkan stop app seperti panduan upgrade 000006. Perubahan `.env` membutuhkan recreate container dengan `docker compose -f docker-compose.dev.yml up -d --force-recreate app`. Perubahan `.air.toml` membutuhkan restart app; perubahan Dockerfile membutuhkan build ulang.

```sh
make dev-logs
make dev-migrate
make dev-down # volume data tetap tersimpan
```

## Keamanan aplikasi

Lihat [catatan audit dan batas perlindungan](docs/security.md). Login karyawan sekarang wajib `{fullname,nik}`. Cookie menjadi satu-satunya tempat token dikembalikan; body login/refresh tidak lagi memuat `access_token`/`refresh_token`. Foto tidak boleh di-cache, dan gambar baru dinormalisasi untuk membuang metadata/trailer. Go minimum menjadi **1.27.3** untuk memakai runtime yang sudah ditambal.

Compose production memaksa cookie Secure dan APP_ENV production, sehingga membutuhkan HTTPS. Startup menolak secret contoh dan password admin seed. Sebelum pertama menjalankan app production: jalankan DB dan migration, ganti hash password admin melalui koneksi DB tepercaya (bcrypt/pgcrypto), isi secret acak, lalu jalankan app. Compose dev tetap bisa dipakai lewat HTTP lokal untuk pengembangan.

## Transaksi dan request bersamaan

Check-in, checkout, permintaan pulang awal, dan submit laporan kerja memakai advisory lock PostgreSQL per employee_id karyawan di dalam transaksi. Pemeriksaan status, metadata foto, counter ID, dan perubahan absensi/laporan menggunakan transaksi yang sama. Request karyawan lain bisa tetap berjalan; beberapa instance API memakai lock database yang sama.

Import pegawai memakai transaksi dan advisory lock khusus import agar dua import, termasuk `deactivateMissing`, tidak menghasilkan gabungan status yang tidak konsisten. Unique constraint dan conditional update tetap menjadi pelindung terakhir untuk data ganda. Insert laporan juga mengunci dan memeriksa baris attendance yang masih terbuka.

Lock ditunggu maksimal 3 detik; statement maksimal 15 detik dan transaksi request dibatasi 30 detik. Lock timeout/deadlock dikembalikan sebagai HTTP 503 `operation_busy`, duplicate attendance sebagai 409; client dapat mencoba lagi dengan jeda. Callback upload tidak otomatis diulang karena stream file dapat sudah terbaca. Lock dilepas saat commit/rollback, termasuk saat request dibatalkan atau panic yang ditangani middleware.

File baru dibersihkan saat rollback yang diketahui. Commit database dan filesystem tetap tidak atomik bersama; proses mati mendadak atau commit dengan hasil tidak pasti membutuhkan rekonsiliasi file. Logout menggunakan access cookie bertanda tangan untuk mengenali sesi meskipun refresh cookie baru saja dirotasi request lain.

Tes integrasi concurrency memakai dua pool koneksi terpisah dan `go test -race`: ID paralel, check-in/checkout, submit berulang, report-versus-checkout pada kedua urutan, import bersamaan, refresh-versus-logout, rollback foto, panic, dan lock timeout. Gunakan database uji yang sudah menjalankan migration/pgcrypto:

```sh
TEST_DATABASE_URL="...database uji..." go test -race ./...
```

Referensi penguncian: [PostgreSQL explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html).

## Kapasitas 1500 karyawan

Baseline satu instance API untuk VPS 8 vCPU / RAM 8 GB (tetap harus diuji pada mesin tujuan):

| Environment | Default | Fungsi |
|---|---:|---|
| `DB_MAX_CONNS` | 32 | Maksimum koneksi DB per instance API |
| `DB_MIN_CONNS` | 4 | Koneksi minimum pool |
| `EMPLOYEE_LOGIN_PER_IP` | 3000/menit | Mendukung satu IP/Wi-Fi bersama; limit lima percobaan per akun/IP tetap berlaku |
| `UPLOAD_WORKERS` | 4 | Batas decode/re-encode gambar bersamaan |
| `MAX_ACTIVE_UPLOADS` | 8 | Batas upload aktif sebelum multipart dibaca |

Admin tetap punya budget login terpisah 100/menit/IP. Admission upload menunggu maksimal lima detik, kemudian mengembalikan 503 dan `Retry-After: 2` tanpa membaca seluruh body. Frontend harus mempertahankan status belum terkirim dan menyediakan retry dengan backoff/jitter untuk 429/503; ini bukan respons sukses absensi. Jangan membuka 1500 koneksi DB untuk 1500 karyawan. Bila API ditambah replika, jumlah seluruh pool harus disesuaikan dengan anggaran koneksi PostgreSQL.

Index migration `000005_employee_login_index` mempercepat pencarian nama aktif untuk login. Terapkan migration dan rebuild app setelah perubahan ini.

Hasil dan cara mengulang benchmark: [load test 1500 karyawan](load-tests/README.md). Hasil lokal tersebut bukan jaminan SLA VPS; ukuran foto, disk, jaringan, CPU yang dibagi dengan tenant lain, dan request admin juga memengaruhi hasil. Tidak diperlukan Redis untuk baseline satu instance ini; limiter bersama tetap diperlukan bila deployment memakai beberapa replika.

## Satu DB untuk Compose utama dan dev

Kedua file menetapkan `name: absensi-dev`. `docker compose run --rm migrate` dan `make dev-migrate` sekarang mengarah ke service DB yang sama. DB lokal tetap `localhost:${DB_PORT}`, sedangkan koneksi antar-container tetap `db:5432`. Nama project Docker tidak menentukan APP_ENV: Compose utama tetap production, Compose dev tetap development.

Data aktif tetap berada pada `absensi-dev_pgdata`; foto pada `absensi-dev_storage`. Container lama `absensi-api-db-1` dapat dihentikan/dihapus tanpa opsi volume. Volume `absensi-api_pgdata` lama tidak otomatis digabung atau dihapus; simpan sampai yakin tidak ada data yang perlu diambil.

Untuk menjalankan dev:

```sh
make dev-migrate
make dev
```

Hindari `down -v` pada project aktif karena opsi itu menghapus volume data. Jika berganti image dev/production di mesin yang sama, ownership file foto pada volume perlu sesuai user non-root image production; file PostgreSQL tetap dikelola image PostgreSQL yang sama.

CRUD master bidang tersedia khusus admin di `/api/v1/admin/bidang` (POST/GET list, GET/PUT/DELETE `/{bidang_id}`). Employee CRUD memakai `bidang_id` format BID, sedangkan import tetap menerima nama bidang. Detail: [bidang](docs/bidang-api.md) dan [employee](docs/employees-api.md).

Bidang mendukung XLSX: `GET /api/v1/admin/bidang/export`, `GET /api/v1/admin/bidang/import/template`, dan `POST /api/v1/admin/bidang/import` (multipart field `file`). Hasil import memuat jumlah dan detail per baris `success`, `skip`, `error`. Batas 8 MiB / 5.000 baris; lihat [panduan bidang](docs/bidang-api.md#import-dan-export-excel).

Mitra memiliki CRUD admin serta XLSX yang sama: base `/api/v1/admin/mitra`, export `/export`, template `/import/template`, import POST `/import`. Excel cukup satu kolom **NAMA MITRA**, hasil import `success`/`skip`/`error`. Detail di [panduan mitra](docs/mitra-api.md).

### Rekap PDF absensi

`GET /api/v1/admin/attendances/recap` memerlukan cookie admin. Parameter: `from`, `to`, `mitra`, `bidang`, `q` (nama), `status` (hadir/bekerja/pulang/tinjau/tanpaPulang/belum), dan `include_absent=true` untuk menyertakan tanggal tanpa catatan. Semua pengambilan absensi dibatasi maksimal 31 hari inklusif. Rentang lebih panjang ditolak dengan pesan pengambilan maksimal satu bulan.

Rekap menghasilkan satu PDF A4 dengan satu halaman per karyawan/tanggal; mencakup identitas, status, jam WIB, koordinat, selfie dan tiga foto pekerjaan. PDF ditulis langsung per halaman. Foto di-resize maksimal 640 px/quality 70 hanya untuk PDF; file asli tidak berubah. Satu proses ekspor PDF per instance aplikasi, request paralel menerima 429. API biasa tetap memakai timeout normal; rekap diberi batas tulis satu jam. Klien menolak PDF terputus/tanpa penutup agar tidak menyimpan laporan yang tidak lengkap.

Batas sebulan bukan jaminan ukuran file atau durasi: 1.500 karyawan x 31 hari dapat menghasilkan 46.500 halaman. Uji kapasitas pada VPS produksi sebelum menyatakan aman pada konfigurasi 8 vCPU/8 GB, dan gunakan filter Mitra/Bidang/nama untuk memperkecil laporan bila diperlukan. Browser/proxy dan ruang penyimpanan unduhan juga memengaruhi proses. Rekap tidak membuat file sementara di storage aplikasi.

Rekap PDF absensi wajib memilih satu Mitra dan mencakup semua bidang dalam mitra tersebut; filter Bidang diabaikan untuk rekap. API juga memvalidasi Mitra sebelum mengambil data. Export Excel tetap wajib memilih satu Mitra dan satu Bidang. Batas 31 hari tetap berlaku.

## Deployment VPS

Untuk production alotive.com, gunakan [panduan deploy VPS](docs/deploy-vps.md): Compose terpisah, API port 8087, network absen, MinIO private, dan publish image GitHub Actions dengan tag SHA.

### Login pegawai dengan NIK KTP

Terapkan migration `000011_employee_nik` sebelum menjalankan versi ini. Admin mengisi NIK KTP asli 16 digit yang unik pada form karyawan atau import Excel; NIK dapat diedit dan disertakan pada export/template. Data lama tetap belum memiliki NIK sampai dilengkapi. Migrasi mencabut sesi pegawai; sesi admin tetap berlaku. Login pegawai sekarang menggunakan `fullname` + `nik`, tanpa nomor HP. Detail format import, perubahan NIK tanpa kehilangan riwayat, dan response ada di [API karyawan](docs/employees-api.md#import-dan-perpindahan-login-ke-nik).

Absen pulang biasa hanya dapat dilanjutkan setelah seluruh sesi laporan kerja lengkap. Jika belum lengkap, tampil notifikasi kuning tanpa pilihan alasan. API juga menolak upaya melewati aturan dengan mengirim alasan langsung. Permintaan Minta Pulang Lebih Awal tetap tersedia; setelah dikirim, pegawai dapat absen pulang dengan alasan dari permintaan tersebut.
