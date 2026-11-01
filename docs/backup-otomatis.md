# Backup malam dan tarik otomatis ke laptop

Mode **S3/Cloudflare R2** juga didukung; foto baru diambil dari bucket private lewat `photoctl` pada image app. Rebuild image setelah upgrade tool. Untuk R2, jeda app hanya sampai metadata/dump database selesai, lalu app hidup kembali saat foto remote disalin. Jangan jalankan penghapusan/lifecycle bucket selama backup. Panduan dan migrasi sumber local: [Cloudflare R2](cloudflare-r2.md).

Konfigurasi siap isi ada di `ops/backup/`. **Belum dipasang/diaktifkan di server atau laptop**. Script memakai Python 3.9+ standard library, Docker Compose pada server, serta SSH dan rsync pada server/laptop. Tidak ada password/token di config; SSH memakai key dan host key yang sudah dipercaya.

## Cara kerja

- Timer Linux server berjalan **02.00 WIB**. API dihentikan sementara agar snapshot konsisten. Database selalu dibackup penuh; hanya foto baru/objek yang belum tersedia disalin dari volume. API dinyalakan kembali setelah snapshot, termasuk saat gagal. Lama jeda bergantung pada jumlah foto baru/dump; semua penulis lain/replika harus ikut berhenti.
- Foto disimpan satu kali sebagai objek SHA-256 di `objects/`. Setiap `snapshots/<timestamp>/` berisi database.dump penuh dan snapshot.json yang memetakan semua foto, path asli, nama karyawan, tanggal, serta jenis foto. Foto lama digunakan kembali; snapshot lama tetap dapat dipulihkan. Foto yang byte-nya sama berbagi objek yang sama.
- Objek baru di-hash saat disalin. Objek lama dicek keberadaan/ukurannya pada server, dengan asumsi file storage immutable sesuai API. Pemeriksaan checksum seluruh foto dilakukan oleh `verify`, termasuk setelah penarikan ke laptop; tidak membaca ulang seluruh byte foto di VPS setiap malam.
- Laptop Mac menarik repository melalui SSH pukul **03.00 menurut zona waktu laptop** dan mencoba lagi tiap jam/saat job dimuat. Gunakan zona waktu Asia/Jakarta bila ingin 03.00 WIB. Laptop harus menyala/terhubung untuk transfer; jadwal tidak membangunkan laptop. Bila tertunda, penarikan berikutnya mencakup snapshot yang belum tersalin.
- rsync melewati file yang sudah sama dan tidak memakai `--delete`. Transfer gagal tidak diberi penanda sukses baru. Snapshot lengkap diterbitkan atomik; folder sementara dikecualikan dari transfer. Penanda terakhir yang lolos checksum ada di `last-verified.json` laptop; periksa timestamp-nya, bukan keberadaan file saja.

Format ini berbeda dari tool full `backup.py`. Foto di repository tetap berupa objek untuk deduplikasi, tetapi bisa diekspor menjadi folder nama manusia tanpa Docker dari laptop. Snapshot hanya mencakup foto yang tercatat di database; file orphan yang tidak dirujuk DB tidak diperlukan untuk restore dan tidak diambil oleh mode incremental. Tool full sebelumnya tetap dapat dipakai untuk salinan seluruh filesystem.

## 1. Konfigurasi server

Contoh project di `/srv/absensi-api`. Sesuaikan path di JSON **dan** unit service jika project berbeda.

```sh
sudo mkdir -p /etc/absensi
sudo chmod 700 /etc/absensi
sudo cp ops/backup/server.example.json /etc/absensi/backup-server.json
sudo chmod 600 /etc/absensi/backup-server.json
```

Isi `repository` dengan folder repository privat di luar volume storage aplikasi, misalnya `/srv/backups/absensi`. Isi `compose_file` dengan path absolut ke Compose yang benar. Gunakan Compose production pada VPS. Snapshot pertama menyalin semua foto yang ada; snapshot berikutnya hanya yang baru.

Jalankan manual sekali dan pastikan app kembali healthy:

```sh
sudo python3 scripts/backup_incremental.py nightly --config /etc/absensi/backup-server.json
sudo python3 scripts/backup_incremental.py verify --repository /srv/backups/absensi
docker compose -f docker-compose.yml ps
```

Template service berjalan sebagai root. Akun SSH untuk menarik backup harus dapat membaca repository privat 0700 tersebut. Pilihan yang sesuai deployment adalah menjalankan unit sebagai user operator yang sama (ubah `User=`), beri akses Docker dan kepemilikan repository/config pada user itu. Akses Docker setara hak administratif host; jangan membuka backup ke semua user dengan chmod 777.

Aktifkan setelah config/path dan hasil uji benar:

```sh
sudo cp ops/backup/absensi-backup.service ops/backup/absensi-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now absensi-backup.timer
systemctl list-timers absensi-backup.timer
journalctl -u absensi-backup.service --since today
```

`Persistent=true` membuat timer mengejar run yang terlewat saat server dinyalakan. Untuk menonaktifkan jadwal: `sudo systemctl disable --now absensi-backup.timer`. Lock `.backup.lock` pada project dan `.nightly.lock` pada repository mencegah overlap dengan backup lain. Lock kernel dilepas otomatis ketika proses berhenti/crash; file lock boleh tetap ada dan tidak perlu dihapus. Setelah host mati mendadak, periksa app sebelum menjalankan ulang. Jangan menghapus file lock saat ada proses aktif.

## 2. Konfigurasi laptop

Salin `ops/backup/client.example.json` ke `backup-client.json` (sudah diabaikan Git). Isi:

```json
{
  "ssh_host": "user@alamat-server",
  "ssh_port": 22,
  "identity_file": "~/.ssh/id_ed25519",
  "remote_repository": "/srv/backups/absensi",
  "local_repository": "~/Backups/absensi"
}
```

Remote path harus absolut tanpa spasi; tujuan lokal boleh memakai spasi. Gunakan akun SSH yang dapat membaca repository, dan pasang public key pada server. Private key tidak dikirim/disimpan di project. Cek fingerprint server melalui saluran tepercaya saat pertama kali `ssh user@alamat-server`; script mempertahankan `StrictHostKeyChecking=yes` dan tidak mengizinkan login password interaktif. Key ber-passphrase harus tersedia melalui SSH agent saat jadwal berjalan.

```sh
chmod 600 backup-client.json
python3 scripts/backup_incremental.py pull --config /path/absolut/backup-client.json
```

Setelah pull sukses, script memeriksa checksum database dan **seluruh foto pada snapshot terbaru lokal**. File offline tidak dipromosikan menjadi "verified" jika rsync/checksum gagal. Tidak ada penghapusan data server atau file lama di laptop. Nama snapshot yang berhasil diverifikasi dicetak dalam log.

### Jadwal Mac (launchd)

Salin `ops/backup/id.absensi.backup-pull.plist` ke `~/Library/LaunchAgents/`. Ganti semua `/ABSOLUTE/PATH/...` dengan path sebenarnya, termasuk interpreter Python 3.9+ (cek `command -v python3`), script, config dan file log. Buat folder log terlebih dahulu. Path di plist harus absolut; `~` tidak diekspansi oleh launchd.

```sh
plutil -lint ~/Library/LaunchAgents/id.absensi.backup-pull.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/id.absensi.backup-pull.plist
launchctl kickstart gui/$(id -u)/id.absensi.backup-pull
```

Periksa log dan `last-verified.json`. Untuk menonaktifkan: `launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/id.absensi.backup-pull.plist`. Tidak ada job yang otomatis dipasang oleh script Python. Bila rsync lama bawaan laptop gagal, gunakan rsync yang tersedia pada PATH job; konfigurasi tidak memakai opsi `--protect-args` yang tidak tersedia pada rsync lama.

## Ambil foto bulan tertentu

Jalankan di laptop setelah pull. Perintah ini hanya membangun arsip bulan yang dipilih, bukan seluruh tahun:

```sh
python3 scripts/backup_incremental.py export-month \
  --repository ~/Backups/absensi --snapshot latest \
  --month 2026-01 --output ~/Backups/export-januari-2026
```

Hasil: `photos.tar.gz`, `photos.csv`, dan `manifest.json`. Foto di arsip bernama `2026-01/EMP-..._Nama-Karyawan/2026-01-05/masuk.jpg`, `laporan-1.jpg`, dst. Ekstrak arsip ke folder kosong untuk dibaca/dikirim. Tidak memerlukan VPS aktif. Jika data Januari sudah dihapus dari database terbaru, pilih ID snapshot lama yang masih memuat Januari, bukan `latest`.

## Restore aplikasi

Pilih ID snapshot dari folder `snapshots`, kemudian bangun bundle lengkap di laptop/disk tujuan:

```sh
python3 scripts/backup_incremental.py materialize \
  --repository ~/Backups/absensi --snapshot latest \
  --month 2026-01 --output ~/Backups/restore-absensi
python3 scripts/backup.py verify ~/Backups/restore-absensi
```

Bundle berisi database.dump + storage.tar.gz + arsip bulanan, kompatibel dengan [panduan restore ke target kosong](backup-restore.md#restore-ke-lingkungan-baru-dan-kosong). `--month` hanya memilih arsip yang mudah dibaca; database dan storage untuk restore mencakup **seluruh snapshot**. Pastikan ruang kosong cukup saat membangun bundle ini.

Simpan secrets `.env` asli terpisah dalam media terenkripsi/password manager, terutama PHONE_HASH_SECRET. Tidak ada salinan secrets dalam repository backup. File backup belum dienkripsi otomatis: gunakan disk terenkripsi dan SSH. Checksum mendeteksi kerusakan, bukan tanda tangan pengirim.

## Kapasitas dan batasan

Incremental menghindari penyalinan foto yang sama tiap malam, **bukan menghapus kebutuhan storage**. Jika repository berada pada disk VPS yang sama, foto aplikasi + repository memakai kira-kira dua salinan, ditambah dump DB harian. Untuk VPS 60 GB, repository sebaiknya di disk/mount terpisah dan tetap dipantau; salinan lokal laptop harus terverifikasi sebelum keputusan cleanup.

Belum ada prune/retensi otomatis: snapshot lama/objek tidak pernah dihapus oleh tool ini. Jangan menghapus folder objects berdasarkan umur karena snapshot lama bisa memakainya. Kebijakan retensi membutuhkan pelacakan referensi snapshot serta uji restore. Hilangnya VPS sebelum penarikan berikutnya tetap dapat menghilangkan backup yang belum tersalin; jadwal ini bukan PITR/backup real-time. Tidak ada klaim backup sukses bila laptop belum terhubung dan verifikasi belum selesai.

Tes tanpa data asli:

```sh
python3 -m unittest discover -s scripts -p 'test_backup*.py' -v
BACKUP_DOCKER_TEST=1 python3 -m unittest discover -s scripts -p 'test_backup*.py' -v
```
