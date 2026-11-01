# Cloudflare R2 private (S3-compatible)

Adapter memakai AWS SDK for Go v2 dengan static S3 credentials, endpoint R2, dan region `auto`, mengikuti [contoh resmi Cloudflare](https://developers.cloudflare.com/r2/examples/aws/aws-sdk-go/). PutObject menggunakan conditional `If-None-Match: *` agar object yang sudah ada tidak ditimpa; R2 mendukung conditional create secara atomik menurut [kompatibilitas S3 R2](https://developers.cloudflare.com/r2/api/s3/api/).

Mode `STORAGE_DRIVER=s3` mengirim foto baru ke bucket tujuan. Development sekarang dapat memakai [MinIO lokal](minio-development.md), sedangkan mode `local` tetap tersedia untuk pemulihan. R2 tidak memerlukan server MinIO. Foto local lama perlu disalin sebelum switch; foto yang sudah di MinIO dapat dipindahkan melalui snapshot/materialize ke staging lalu migrasi ke R2.

## Siapkan bucket dan credentials

1. Aktifkan R2 pada akun Cloudflare dan buat **private bucket** `absensi-production`.
2. Public Development URL `r2.dev` dan custom domain publik bucket harus **dinonaktifkan**. Browser mengambil foto melalui API dengan cookie admin, bukan langsung dari bucket.
3. Buat R2 S3 API credentials dengan **Object Read & Write**, dibatasi hanya ke bucket ini. Akses delete diperlukan untuk kompensasi upload bila transaksi DB gagal. Tidak perlu memberikan hak admin seluruh akun.
4. Simpan **Access Key ID** dan **Secret Access Key** pada `.env` API. Ini bukan Bearer API Token Cloudflare; SDK memakai pasangan S3 credentials. Jangan kirim ke chat/frontend atau commit ke repository.

Gabungkan template [ops/cloudflare-r2.env.example](../ops/cloudflare-r2.env.example) ke `.env` API:

```dotenv
STORAGE_DRIVER=s3
S3_ENDPOINT=https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET=absensi-production
S3_PREFIX=production
S3_ACCESS_KEY_ID=ISI_DI_SERVER
S3_SECRET_ACCESS_KEY=ISI_DI_SERVER
S3_FORCE_PATH_STYLE=true
S3_ALLOW_MINIO_HTTP=false
```

Gunakan S3 endpoint yang diberikan dashboard, tanpa trailing `/bucket`, query, atau embedded username/password. Endpoint production wajib HTTPS. Ganti placeholder account ID. Region untuk provider selain R2 dapat disesuaikan. Prefix opsional; setelah ada data, jangan mengganti bucket/prefix begitu saja karena database menyimpan key relatif terhadap konfigurasi itu.

Frontend tidak memerlukan perubahan env/credentials/CORS bucket. API `/api/v1/admin/photos/PHO-...` tetap melewati guard admin dan mengirim byte asli dengan Content-Type serta no-store. Tampilan foto, unduh foto, PDF individu dan rekap tetap memakai service foto yang sama. Error R2 seperti permission/network ditampilkan sebagai storage tidak tersedia, bukan disamarkan sebagai foto hilang. Health endpoint umum tidak melakukan probe R2; uji upload dan pembacaan foto tetap diperlukan saat aktivasi.

## Aktivasi jika belum ada foto lama

```sh
docker compose -f docker-compose.yml up -d --build --no-deps app
```

Development: ganti dengan `docker-compose.dev.yml`. Kedua Compose sudah membaca `.env` melalui `env_file`, jadi tidak ada default credential atau container storage tambahan. Rebuild diperlukan agar binary `photoctl` ikut tersedia, termasuk mode Air; hot reload Go saja tidak membangun ulang tool operator di image.

Uji selfie masuk/laporan/pulang dari akun uji, lalu buka foto dari admin dan uji PDF/unduh. Pastikan object muncul di bucket/prefix yang benar dan tidak dapat diakses publik. Aplikasi tidak melakukan fallback diam-diam ke local bila R2 gagal.

## Migrasi foto lama dari volume Docker

Mengganti endpoint/credentials **tidak otomatis memindahkan data**. Tidak ada migration SQL karena `photos.storage_key` dan PHO ID tetap sama. Tool di bawah menyalin byte persis ke key yang sama (ditambah prefix bucket), lalu membaca ulang dan membandingkan SHA-256. File lokal dan database tidak diubah/dihapus.

1. Selagi mode lama masih local, buat dan verifikasi backup penuh sesuai [panduan backup](backup-restore.md), lalu salin ke luar VPS.
2. Isi `.env` R2, build image baru, lalu hentikan app agar tidak ada upload baru selama migrasi:

   ```sh
   docker compose -f docker-compose.yml build app
   docker compose -f docker-compose.yml stop app
   ```

3. Dry run (tidak mengupload/menghapus object):

   ```sh
   docker compose -f docker-compose.yml run --rm --no-deps --entrypoint photoctl app \
     migrate-local --local-dir /app/storage
   ```

4. Jika sumber dan akses bucket benar, jalankan penyalinan:

   ```sh
   docker compose -f docker-compose.yml run --rm --no-deps --entrypoint photoctl app \
     migrate-local --local-dir /app/storage --apply
   ```

5. Jalankan dry run lagi: `pending` harus 0 dan object existing terverifikasi. Tool menolak object dengan isi berbeda, tidak menimpanya. File lokal hilang/ukuran tidak sesuai DB juga membatalkan proses. Penyalinan yang terputus bisa dijalankan ulang; object identik dilewati.
6. Jalankan app baru:

   ```sh
   docker compose -f docker-compose.yml up -d --no-deps app
   ```

Gunakan Compose yang **sama dengan deployment sumber** untuk seluruh langkah agar volume/permission cocok. Jangan `down -v`, menghapus volume, atau menyalakan instance API lama bersamaan. Jika migrasi gagal, file lokal masih ada; perbaiki dulu atau kembalikan `STORAGE_DRIVER=local` dan recreate app untuk menggunakan sumber lama. Jangan hapus data lokal sampai uji foto lama, upload baru, unduh, PDF, serta backup R2 berhasil. Object yang sempat terupload tetapi belum terpakai tetap private.

## Backup malam, arsip bulanan dan restore

`backup_incremental.py nightly` mendeteksi driver dari container app. Untuk S3/R2, tool memakai binary `photoctl export` pada image app untuk membaca **hanya foto baru** dari bucket private; tidak menyalin volume kosong sebagai backup. Secret S3 diteruskan melalui file env sementara 0600, bukan argument CLI/manifest. JWT_SECRET, PHONE_HASH_SECRET, dan DATABASE_URL tidak diberikan ke helper export.

Database + daftar foto diambil saat app dihentikan. Untuk S3, app dinyalakan kembali **sebelum download foto dari R2**, karena objek aplikasi menggunakan create-only key. Tidak boleh ada proses/lifecycle eksternal yang menghapus atau mengganti object selama backup. Snapshot baru baru diterbitkan setelah semua foto yang diperlukan tersedia dan diverifikasi ukurannya. Pemeriksaan seluruh checksum dilakukan saat verify/pull ke laptop.

Konfigurasi timer 02.00 WIB, pull SSH ke laptop, export bulan tertentu, dan materialize bundle restore tetap seperti [backup otomatis](backup-otomatis.md). Mode manual lama `backup.py backup` hanya untuk local dan **menolak S3/R2** agar tidak menghasilkan backup foto kosong. Gunakan nightly lalu materialize jika perlu bundle penuh.

Repository backup masih memakan disk jika disimpan di VPS: walau upload aktif ada di R2, snapshot foto backup menjadi salinan lokal di lokasi repository. Arahkan repository ke disk/mount backup terpisah atau pantau kapasitas dan penarikan ke laptop. R2 sendiri tidak menggantikan backup terpisah. Tidak ada cleanup bucket/VPS otomatis.

Restore ke target kosong dapat menggunakan `STORAGE_DRIVER=local` dengan database.dump + storage.tar.gz seperti panduan lama. Untuk target R2/bucket/provider baru, restore DB serta file asli pada volume staging, jalankan `photoctl migrate-local --apply` ke bucket tujuan, verifikasi, baru aktifkan mode S3. Simpan secrets aplikasi asli, khususnya PHONE_HASH_SECRET, secara terpisah; credentials R2 dapat diganti sesuai bucket tujuan. Path relatif foto dan business ID tidak berubah.

## Batas operasional dan tes

Request storage memiliki timeout dan retry terbatas; bucket lambat/tidak tersedia menghasilkan error terkontrol. DB dan object storage tidak dapat melakukan commit atomik bersama: kompensasi menghapus upload saat rollback yang diketahui, tetapi crash/commit ambigu masih dapat meninggalkan orphan. Jangan mengaktifkan lifecycle yang menghapus foto aplikasi sebelum kebijakan retensi/backup terverifikasi dibuat. Pembatasan R2, akses akun/token, serta monitoring storage perlu dikonfigurasi pada deployment.

Tes lokal tidak memakai bucket/credentials asli:

```sh
go test -race ./internal/pkg/storage ./internal/pkg/config ./internal/domain/photos ./cmd/photoctl
python3 -m unittest discover -s scripts -p 'test_backup*.py' -v
# Opt-in Docker test: endpoint S3 tiruan + DB terisolasi + binary production.
docker build -t absensi-r2-verify:local .
BACKUP_DOCKER_TEST=1 BACKUP_S3_TEST_IMAGE=absensi-r2-verify:local \
  python3 -m unittest discover -s scripts -p 'test_backup*.py' -v
```

Tes mencakup signature S3, key/prefix, conditional create paralel, read/delete, cancel, private response, kompensasi gagal DB, dry-run/copy identik/mismatch, export tar, snapshot incremental remote dan materialize/restore. Koneksi akun Cloudflare nyata baru dapat diverifikasi setelah credentials diisi.
