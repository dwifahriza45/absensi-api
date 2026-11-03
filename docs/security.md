# Catatan keamanan — 31 Oktober 2026

Audit ini mencakup kode API, upload/download foto, session, input HTTP, dependency Go, dan container aplikasi. Ini merupakan pemeriksaan kode dan tes regresi terarah, bukan jaminan bebas semua celah atau pengganti penetration test deployment.

## Perbaikan

- Karyawan wajib nama lengkap + nomor HP terdaftar; nomor dibandingkan melalui HMAC constant-time. Nama saja tidak cukup untuk login. Nama ganda tetap ditolak agar tidak memilih akun orang lain.
- Token login/refresh tidak dikembalikan dalam JSON. Cookie memakai HttpOnly, SameSite Lax, path terpisah per role, dan Secure wajib di production. Logout mencabut sesi di DB, refresh merotasi token, akun nonaktif ditolak pada akses dan refresh.
- Header Authorization wajib berformat Bearer. Route foto selalu melalui guard admin. UUID/kode berurutan bukan pengganti otorisasi.
- Mutation dengan Origin asing/null ditolak. Request browser cross-site tanpa Origin juga ditolak melalui Sec-Fetch-Site. Non-browser client tanpa Origin tetap diperbolehkan dengan autentikasi yang sesuai.
- Foto dibatasi 5 MiB, maksimum 12 juta piksel dan sisi 8192 piksel. DecodeConfig memeriksa ukuran sebelum decode penuh. Hanya JPEG/PNG yang dapat didecode penuh diterima, lalu di-encode ulang untuk membuang metadata dan data tambahan. Maksimal empat proses validasi gambar bersamaan secara default (`UPLOAD_WORKERS`, batas konfigurasi 1..8). Admission gate bersama membatasi delapan request upload aktif sebelum body multipart dibaca; request lain menunggu maksimal lima detik, lalu mendapat 503/Retry-After.
- Storage local memakai os.Root, key relatif tervalidasi, proteksi symlink keluar root, file 0600 dan direktori 0700. Adapter S3/R2 menggunakan bucket private, signed requests, HTTPS production, conditional create agar object tidak ditimpa, timeout/retry terbatas, serta credentials hanya pada API. Pengecualian HTTP harus diaktifkan eksplisit dengan S3_ALLOW_MINIO_HTTP dan hanya menerima service internal persis minio:9000; endpoint HTTP eksternal tetap ditolak. Tidak ada URL bucket/presigned URL publik di respons frontend. File/direktori lama tidak otomatis diubah permission-nya. Detail aktivasi dan migrasi: [R2](cloudflare-r2.md).
- Respons API/foto memakai no-store, nosniff, no-referrer, CSP dan larangan framing. Header no-store mencegah penyimpanan cache respons baru; file yang sudah pernah diunduh atau cache lama tidak dapat dicabut dari perangkat pengguna.
- JSON menolak field asing dan body JSON tambahan, dengan batas ukuran. Angka NaN/Inf ditolak. Query SQL memakai parameter, termasuk lookup ID.
- Reservasi percobaan login atomik: lima per akun/IP selama 15 menit, ditambah 3000 request login karyawan per IP per menit secara default (`EMPLOYEE_LOGIN_PER_IP`), terpisah dari limit admin 100 per IP per menit. Entri limiter dibersihkan dan dibatasi 10.000 entri per limiter.
- Production menolak cookie insecure, secret contoh/pendek, dan admin aktif yang memakai password seed Admin123!. Compose production memakai filesystem read-only, volume storage terpisah, pengguna non-root, tanpa capabilities dan no-new-privileges. File .env varian dan backup tidak ikut build context.
- Runtime Go dan dependency JWT/pgx/x/crypto/x/text diperbarui setelah scan menemukan versi lama yang rentan.

## Verifikasi

Tes mencakup traversal/symlink, overwrite dan permission storage, gambar palsu/truncated/ukuran piksel berlebihan, pembuangan trailer, login tanpa/salah HP, token JSON/cookie, Origin/Fetch Metadata, JSON tambahan, angka non-finite, limiter konkuren, role, refresh replay, logout, dan relasi/migration ID bisnis.

```sh
go test -race ./...
go vet ./...
TEST_DATABASE_URL="...database uji..." go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Scan dengan Go 1.27.3 setelah upgrade tidak menemukan kerentanan pada kode/paket yang dipanggil API. Advisory GO-2026-5932 masih muncul pada level module golang.org/x/crypto karena paket OpenPGP yang tidak dipakai; API memakai bcrypt. Hasil scanner hanya berlaku untuk versi dan database advisory saat pemeriksaan. Air/tool development dan paket OS container tidak tercakup scan dependency API tersebut.

## Batas yang tetap perlu diperhatikan

Nama + nomor HP bukan bukti kepemilikan nomor: orang yang mengetahui keduanya masih bisa mencoba masuk. Ini mengikuti keputusan produk; belum ada OTP atau autentikasi perangkat. Login/search karyawan tetap memiliki endpoint pencarian nama publik dengan hasil terbatas.

Limiter masih dalam memori per proses dan memakai IP koneksi langsung. Pada reverse proxy, semua client dapat terlihat memakai IP proxy yang sama; pengaturan trusted proxy dan rate limit edge perlu ditentukan saat deployment. Header X-Forwarded-For tidak dipercaya begitu saja.

Enkripsi disk/backup, HTTPS di reverse proxy, patch OS, akses operator, firewall, backup/restore, retensi foto, pemantauan disk, dan audit log operasional tetap merupakan tanggung jawab deployment. Gambar lama tidak otomatis dinormalisasi ulang. Pada mutasi absensi, metadata foto dan perubahan absensi berada dalam satu transaksi; file baru dibersihkan saat rollback yang diketahui. Database dan filesystem tetap tidak memiliki commit atomik bersama: proses mati mendadak, commit yang hasilnya tidak pasti, atau gagal cleanup masih dapat menyisakan orphan; rekonsiliasi berkala tetap diperlukan. Pengguna root/administrator host tetap bisa mengakses volume.

Referensi: [OWASP File Upload](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html), [Go traversal-resistant file APIs](https://go.dev/blog/osroot), [advisory OpenPGP yang tidak dipakai](https://pkg.go.dev/vuln/GO-2026-5932).

## Perubahan referensi pada migration 000006

FK antartabel memakai kode bisnis EMP/INT/ATT/PHO, sementara UUID primary key dipertahankan. Sesi menggunakan FK eksplisit USER atau EMP dan kode SES; pemilik yang dihapus mencabut sesi melalui cascade. Constraint mencegah satu sesi dimiliki admin dan karyawan sekaligus. Migrasi mencabut sesi lama tanpa pemilik. JWT lama perlu refresh/login ulang karena subject/jti sekarang memakai kode bisnis. ID berurutan tetap tidak memberi hak akses: guard role/session berlaku untuk setiap request foto dan absensi.

Login pegawai menggunakan nama + NIK KTP 16 digit sejak migration 000011. NIK hanya dikembalikan untuk admin, tidak pada pencarian publik atau profil pegawai. Perubahan profil/import mencabut sesi karyawan terkait.
