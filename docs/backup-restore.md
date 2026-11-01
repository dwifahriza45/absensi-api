# Backup foto dan restore

Tool `backup.py backup` di bawah khusus storage local. Pada R2/S3, tool menolak proses agar tidak membackup volume yang bukan sumber foto. Gunakan `backup_incremental.py nightly` kemudian `materialize` untuk bundle restore yang sama; lihat [panduan R2](cloudflare-r2.md).

Untuk jadwal malam, foto incremental, dan transfer otomatis ke laptop gunakan [backup otomatis](backup-otomatis.md). Panduan di bawah tetap untuk tool snapshot penuh manual serta prosedur restore bundle.

Jalankan dari folder `absensi-api` pada host Docker dengan Python 3.9+ dan Docker Compose. Tidak ada library Python tambahan. Ini tool operator, bukan endpoint publik.

## Backup

```sh
python3 scripts/backup.py backup \
  --month 2026-11 \
  --output /mnt/backup/absensi-2026-11-01 \
  --compose-file docker-compose.yml
```

Untuk development gunakan `docker-compose.dev.yml`. Januari 2026 memakai `--month 2026-01`. Folder output harus baru, di luar volume `/app/storage`; disarankan disk eksternal/mount backup di luar disk VPS.

Script menghentikan **app** dengan graceful timeout 60 detik, mengambil snapshot database dan seluruh storage, lalu menyalakan app kembali jika awalnya berjalan. Database tetap berjalan. Setelah salinan selesai, app dinyalakan sebelum verifikasi lokal. Jika gagal, script mencoba menyalakan kembali app dalam `finally`. Jika host mati mendadak, operator harus memeriksa dan menjalankan `docker compose start app` bila perlu.

Jalankan saat sepi: lama downtime bergantung pada jumlah data dan kecepatan disk. Semua penulis lain (replika, import/manual SQL, cleanup) juga harus berhenti. Script ditujukan untuk deployment **satu service app, satu database, satu storage** seperti Compose project ini. Lock `.backup.lock` mencegah dua backup script pada checkout yang sama; bukan lock lintas mesin. Jangan menghapus lock saat proses backup masih berjalan.

Hasil folder:

| File | Isi |
|---|---|
| `database.dump` | Seluruh database, format custom PostgreSQL |
| `storage.tar.gz` | Seluruh foto dengan path asli untuk restore |
| `photos.tar.gz` | Foto bulan terpilih dengan nama mudah dibaca |
| `photos.csv` | Nama, employee_id, attendance_id, photo_id, tanggal, jenis foto, path asli/arsip, checksum |
| `manifest.json` | Versi, waktu, bulan, ukuran dan SHA-256 komponen serta setiap foto |

Contoh arsip bulanan:

```text
2026-11/
  EMP-20261101-000001_Hamdan-Soleh/
    2026-11-01/
      masuk.jpg
      laporan-1.jpg
      laporan-2.jpg
      laporan-3.jpg
      pulang.jpg
```

Bulan mengikuti **tanggal kerja attendance**, bukan folder upload. Nama memakai data saat backup, employee_id membedakan karyawan bernama sama. Foto yang belum ada tidak dibuatkan file kosong. Backup gagal bila foto yang dirujuk database hilang atau ukuran metadata berbeda. Bulan tanpa foto menghasilkan arsip bulanan kosong, tetapi snapshot tetap lengkap. Orphan ikut snapshot storage namun tidak masuk arsip bulanan.

Snapshot database/storage selalu **seluruh data**, walaupun `--month` satu bulan. Arsip bulanan adalah salinan tambahan: siapkan ruang lebih dari total storage + database. File diproses bertahap, tidak ditampung seluruhnya dalam RAM. Foto tidak dikompresi ulang dan path storage aplikasi tidak diubah.

Output privat 0700/0600, tetapi **belum dienkripsi otomatis**. Gunakan media terenkripsi dan SSH/SFTP. Database dump memuat data privat termasuk hash dan sesi; jangan taruh backup di folder web publik/Git. `.env` tidak disertakan: simpan secrets asli, terutama **PHONE_HASH_SECRET**, terpisah di password manager atau arsip terenkripsi. Kunci yang berbeda membuat nomor HP tidak dapat dipulihkan dengan benar.

## Salin ke laptop dan verifikasi

```sh
# Jalankan dari laptop; sesuaikan SSH host/lokasi.
scp -r user@server:/mnt/backup/absensi-2026-11-01 ./backup-lokal/
python3 scripts/backup.py verify ./backup-lokal/absensi-2026-11-01
```

Verifikasi offline tidak perlu Docker. Checksum mendeteksi file rusak/tidak lengkap, bukan tanda tangan pengirim; gunakan backup dari sumber tepercaya. Ekstrak **photos.tar.gz** ke folder kosong untuk mencari/mengirim foto ke user. Restore aplikasi memakai **storage.tar.gz**, bukan folder bernama manusia.

Script **tidak menghapus data sumber**. Backup pada VPS yang sama belum melindungi dari kehilangan VPS. Jangan cleanup sebelum salinan di luar VPS lolos verifikasi **dan uji restore**. Retensi/jadwal/cleanup otomatis belum diaktifkan. Arsip foto bulanan dapat disimpan terpisah; snapshot penuh tidak perlu ditumpuk setiap bulan selamanya tanpa kebijakan retensi.

## Restore ke lingkungan baru dan kosong

Latihan pada host/Compose terpisah terlebih dahulu. Jangan jalankan pada database/storage aktif. Siapkan `.env` dengan secrets asli serta volume tujuan **baru/kosong**. Jangan migrate/seed sebelum restore; dump sudah mencakup schema, migration version, relasi, dan counter ID. Gunakan PostgreSQL 16 atau versi kompatibel dengan dump.

1. Verifikasi folder backup, buat container app tanpa menjalankannya, hidupkan DB kosong:

   ```sh
   python3 scripts/backup.py verify /mnt/backup/absensi-2026-11-01
   docker compose -f docker-compose.yml create app
   docker compose -f docker-compose.yml up -d db
   ```

2. Tunggu DB healthy, pastikan target kosong, restore seluruh dump:

   ```sh
   docker compose -f docker-compose.yml exec -T db sh -c \
     'pg_restore --exit-on-error --single-transaction --no-owner --no-acl -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
     < /mnt/backup/absensi-2026-11-01/database.dump
   ```

3. Restore path foto asli ke volume kosong, atur pemilik sesuai pengguna app production:

   ```sh
   gzip -dc /mnt/backup/absensi-2026-11-01/storage.tar.gz | \
     docker compose -f docker-compose.yml run --rm -T --no-deps \
       --user 0 --entrypoint sh app -ec \
       'tar -xpf - -C /app/storage; if id app >/dev/null 2>&1; then chown -R app:app /app/storage; fi'
   ```

4. Jalankan `docker compose -f docker-compose.yml start app`. Periksa health, login admin, absensi bulan terkait, foto, unduh foto, dan PDF. Setelah disaster recovery, cabut sesi lama sebelum membuka layanan pengguna. Simpan backup sumber sampai hasil restore terverifikasi.

Arsip bulanan saja tidak cukup untuk restore aplikasi: perlu pasangan snapshot database/storage dan secrets yang sesuai. Snapshot tidak mengembalikan aktivitas setelah waktu backup. Backup harian/incremental atau WAL/PITR adalah konfigurasi operasional terpisah.

## Tes

```sh
python3 -m unittest discover -s scripts -p 'test_backup.py' -v

# Uji backup + restore dengan container PostgreSQL/storage sementara terisolasi:
BACKUP_DOCKER_TEST=1 python3 -m unittest discover -s scripts -p 'test_backup.py' -v
```
