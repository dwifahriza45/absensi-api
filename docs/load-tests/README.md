# Load test 1500 karyawan — 31 Oktober 2026

Pengujian dilakukan pada stack Docker Desktop lokal yang terisolasi, satu API dev dengan Air dan PostgreSQL 16. Docker mendapat 8 CPU arsitektur ARM64 (aarch64) dan RAM 8.217.731.072 byte (~7,65 GiB). Generator beban berada pada host yang sama; angka ini tidak mencakup internet, HTTPS/reverse proxy, latensi seluler, maupun disk VPS tujuan.

Setiap skenario membuat 1500 pegawai berbeda, login dari satu IP bersama, lalu mengirim satu check-in dengan JPEG sintetis 640×480, kualitas 85, ukuran 232.545 byte (~227 KiB). Tidak ada retry otomatis. Kedua skenario memakai akun berbeda, sehingga seluruh check-in merupakan insert baru dengan penyimpanan file dan transaksi database.

| Skenario | Worker client maksimum | Jendela kedatangan | Berhasil | Waktu check-in | p95 check-in |
|---|---:|---:|---:|---:|---:|
| Tersebar | 64 | 60 detik | 1500/1500 | 59,987 detik | 23,11 ms |
| Burst | 128 | 0, antre di generator | 1500/1500 | 7,826 detik | 766,64 ms |

Semua login juga berhasil (1500/1500 per skenario). Endpoint health mengembalikan 200 setelah masing-masing tes. Pemeriksaan DB setelah kedua skenario: 3000 attendance, 3000 pasangan employee/tanggal unik, dan 3000 metadata foto.

**Burst tersebut bukan 1500 koneksi aktif sekaligus:** maksimal 128 worker client, sedangkan API membatasi delapan upload aktif dan empat decode gambar. Latency dihitung setelah penjadwalan kedatangan; throughput menyertakan jendela kedatangan. Detail mentah: [window](2026-10-31-window-1500.json), [burst](2026-10-31-burst-1500.json).

Angka penggunaan RAM/CPU puncak tidak direkam secara kontinu, sehingga laporan ini tidak mengklaim batas maksimum RAM/CPU. Foto 5 MiB/12 megapiksel, 1500 upload serentak, login admin bcrypt massal, serta trafik lain di server belum dibenchmark. Untuk VPS 8 vCPU/8 GB, konfigurasi ini merupakan titik awal yang perlu diuji ulang dengan foto dan kondisi deployment nyata.

## Mengulang pada stack uji terisolasi

Script menulis data dan file; gunakan database/volume khusus tes. Default target localhost, target remote memerlukan `--allow-remote` dan deployment uji. Password admin dibaca dari environment dan tidak dicetak atau disimpan pada laporan.

```sh
go run ./scripts/make_load_photo.go -out /tmp/absensi-load.jpg
# Set LOADTEST_ADMIN_PASSWORD di shell.
python3 scripts/load_attendance.py --allow-writes \
  --count 1500 --workers 64 --window-seconds 60 \
  --photo /tmp/absensi-load.jpg --output /tmp/window.json
python3 scripts/load_attendance.py --allow-writes \
  --count 1500 --workers 128 --window-seconds 0 \
  --photo /tmp/absensi-load.jpg --output /tmp/burst.json
```

Untuk menguji foto lapangan, ganti `--photo` dengan JPEG/PNG uji yang sesuai. Client mendukung worker 1..256; hasil gagal/429/503 dicatat dan menyebabkan exit nonzero, bukan disamarkan sebagai sukses.

Referensi batas koneksi: [PostgreSQL 16](https://www.postgresql.org/docs/16/runtime-config-connection.html), [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool#Config).

## Regresi setelah migration 000006

Tes awal di atas memakai schema versi 000005. Setelah seluruh FK berpindah ke ID bisnis dan sesi memakai SES/USER/EMP, tes burst diulang pada lingkungan lokal yang sama: 1500 akun berbeda, 128 worker, JPEG 232.545 byte, tanpa retry otomatis.

- Login: 1500/1500 berhasil; 0,847 detik total, p95 104,10 ms.
- Check-in + foto: 1500/1500 berhasil; 7,589 detik total, p95 656,62 ms.
- Health setelah tes: 200.

[Laporan mentah schema 000006](2026-10-31-business-keys-1500.json). Batas interpretasi lingkungan lokal, ukuran foto, jumlah worker, dan belum mengukur puncak RAM/CPU tetap sama.
