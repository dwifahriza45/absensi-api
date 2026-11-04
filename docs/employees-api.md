# CRUD karyawan (admin)

Semua endpoint membutuhkan cookie admin dari login. Pakai host yang sama saat login dan mengakses endpoint berikut, misalnya `http://127.0.0.1:8082`.

| Method | Path | Keterangan |
| --- | --- | --- |
| POST | `/api/v1/admin/employees` | Buat karyawan, respons 201 |
| GET | `/api/v1/admin/employees` | List karyawan aktif |
| GET | `/api/v1/admin/employees/{employee_id}` | Detail, termasuk karyawan nonaktif |
| PUT | `/api/v1/admin/employees/{employee_id}` | Ubah data |
| DELETE | `/api/v1/admin/employees/{employee_id}` | Hapus permanen hanya jika belum ada attendance |

Parameter path menggunakan `EMP-YYYYMMDD-000001`, bukan UUID `id`. ID dibuat otomatis oleh database. Mitra harus sudah ada; kirim `mitra_id` format `MTR-YYYYMMDD-000001`, bukan nama atau UUID mitra.

## Create

```json
{
  "name": "Budi Santoso",
  "nik": "0123456789012345",
  "phone": "6281234567890",
  "mitra_id": "MTR-20261031-000001",
  "bidang_id": "BID-20261031-000001"
}
```

`name`, `nik`, `mitra_id`, dan `bidang_id` wajib diisi. `phone` opsional; beberapa karyawan boleh tidak memiliki nomor HP. Batas nama 80 karakter. `bidang_id` wajib dan harus mengacu ke master bidang yang sudah ada. Jika diisi, nomor HP harus valid dan unik; nomor yang sama dalam format `08...` dan `+62...` dikenali sebagai nomor yang sama. NIK wajib tepat 16 digit angka, unik, dan dapat diedit. Nomor HP tetap kontak, disimpan sebagai HMAC untuk pencocokan/keunikan dan ciphertext AES-GCM. Field `nik` dan `phone` dikembalikan pada response CRUD/list/detail admin; pencarian publik dan profil/login karyawan tidak menyertakannya. Default `isActive` adalah true.

## Update

```json
{
  "name": "Budi Santoso",
  "nik": "0123456789012345",
  "mitra_id": "MTR-20261031-000001",
  "bidang_id": "BID-20261031-000002",
  "isActive": true
}
```

PUT membutuhkan nama, NIK, mitra, dan bidang. Bidang yang hilang, kosong, null, atau hanya spasi menghasilkan 400 dengan pesan `valid.bidang_id`. Phone yang tidak dikirim atau kosong mempertahankan nomor sebelumnya. `isActive` yang tidak dikirim mempertahankan status sebelumnya. Menyimpan perubahan profil mencabut seluruh session karyawan agar nama/NIK lama tidak terus dipakai; begitu juga `isActive: false`. Untuk mengganti nomor, kirim field `phone` yang baru.

## List dan delete

Filter list yang tersedia: `q`, `mitra` (nama), `bidang` (nama), dan `bidang_id` (kode BID). Tambahkan `includeInactive=true` untuk menyertakan karyawan nonaktif.

Delete menghapus permanen karyawan hanya bila tidak ada data attendance sama sekali, baik statusnya aktif maupun nonaktif. Semua session dicabut lewat FK cascade. Jika sudah ada attendance, API mengembalikan 409 tanpa mengubah profil, status, session, atau riwayat. List/detail memuat `has_attendance`, `can_delete`, dan `delete_reason` untuk disabled icon/tooltip. Transaksi memakai lock import, lock attendance per EMP, dan row lock employee; FK tetap mencegah relasi putus saat request absen bersamaan. Nonaktifkan/aktifkan karyawan lewat PUT `isActive: false/true`, bukan DELETE.

Write menggunakan transaksi, lock yang sama dengan import, serta batas tunggu database. Konflik NIK (`employee_nik_exists`) atau nomor HP menghasilkan 409, mitra tidak ditemukan/validasi input 400, employee tidak ditemukan 404. Format response mengikuti `code`, `status`, `message`, `data`, `error`, dan `valid` untuk validasi field.

## Nomor lama dan backup

Migration 000007 menambahkan `phone_encrypted`. Data lama yang hanya mempunyai hash tidak bisa dipulihkan; isi ulang nomor lewat update atau import. Kunci AES-GCM diturunkan dengan domain terpisah dari `PHONE_HASH_SECRET`, bukan JWT secret. Backup secret ini secara aman bersama backup database, dan jangan menggantinya sembarangan: secret yang hilang membuat nomor tidak bisa dibuka dan pencocokan nomor lama tidak cocok. Ciphertext memakai nonce acak dan autentikasi yang terikat pada hash nomor untuk menolak perubahan atau pertukaran ciphertext antar profil.

Employee response memuat `bidang_id` dan label `bidang` hasil join master. CRUD employee tidak lagi menerima nama bebas `bidang`; import Excel tetap menerima nama pada kolom `bidang` dan mencari/membuat master dalam transaksi. Lihat [CRUD bidang](bidang-api.md).

Import employee juga mewajibkan label `bidang` pada setiap baris. Baris yang kosong dilaporkan sebagai dilewati beserta alasan; bila tidak ada baris valid, import ditolak.

## Export Excel

`GET /api/v1/admin/employees/export` membutuhkan cookie admin. Query opsional: `q` (nama/NIK/nomor HP), `mitra_id`, `bidang_id`, dan `status=active|inactive|all` (default active). Export mencakup seluruh hasil filter, bukan hanya halaman tabel yang tampil.

File `karyawan.xlsx` memiliki sheet Karyawan dengan header NAMA LENGKAP, NOMOR HP, MITRA, BIDANG, NIK, ID KARYAWAN. Header biru tua dengan font putih tebal, huruf besar dan rata tengah. ID KARYAWAN berisi kode EMP yang stabil untuk import ulang; UUID tidak disertakan. NIK ditulis sebagai teks 16 digit; nomor HP ditulis sebagai string dengan format 628...; semua nilai adalah sel string, bukan formula. Kolom status tidak disertakan pada export maupun import. Karyawan baru default aktif; opsi nonaktifkan yang tidak ada di file tetap tersedia pada import.

## Ubah status dari tabel

`PUT /api/v1/admin/employees/{employee_id}/status`, khusus admin, body `{"isActive":false}` atau `{"isActive":true}`. Field wajib bertipe boolean; false valid, null/hilang ditolak. Endpoint ini hanya mengubah status, tidak memerlukan/mengubah nama atau master bidang/mitra. Nonaktifkan mencabut semua session dalam transaksi yang sama; aktifkan kembali tidak menghidupkan session yang sudah dicabut.

Frontend menggunakan ikon power dengan modal konfirmasi. Field Status tidak ada pada form Tambah/Ubah; create default aktif dan edit profil tidak mengirim `isActive`, sehingga status saat ini dipertahankan. Tombol hapus tetap berbeda dan hanya diizinkan tanpa attendance.

Nomor HP pada response dan export menggunakan format `628...` tanpa tanda `+`. Input `08...`, `+62...`, dan `8...` dinormalisasi ke `62...`; lookup hash lama tetap kompatibel. Nomor di Excel ditulis sebagai teks.

## Import dan perpindahan login ke NIK

Migration `000011_employee_nik` menambahkan kolom NIK unik dengan pemeriksaan 16 digit. Nilai karyawan lama tetap NULL sampai admin mengisi NIK asli lewat edit atau import. Migrasi mencabut sesi karyawan agar login ulang menggunakan nama + NIK; sesi admin tetap berlaku. Terapkan migrasi sebelum menjalankan API versi baru, lalu lengkapi NIK karyawan.

Login pegawai menerima `{"fullname":"Budi Santoso","nik":"0123456789012345"}`. API mencari NIK unik dan mencocokkan nama tanpa membedakan kapitalisasi serta spasi berlebih. Nomor HP tidak digunakan sebagai kredensial. NIK tidak disertakan pada pencarian publik maupun profil pegawai.

Import JSON menerima `name`, `nik`, `phone`, `mitra`, `bidang`, dan `employee_id` opsional. Baris NIK tidak valid/duplikat dalam file dilewati beserta alasannya. Untuk mengganti NIK dari hasil export, pertahankan ID KARYAWAN/`employee_id`; karyawan dan riwayat attendance tetap memakai EMP yang sama. ID yang tidak ditemukan ditolak. Tanpa ID, import mencocokkan NIK; hanya untuk karyawan lama yang belum punya NIK, nomor HP yang sama bisa dipakai untuk melengkapi NIK. Konflik NIK dengan karyawan lain menghasilkan 409 dan transaksi dibatalkan. Kolom NIK wajib diisi sebagai teks di Excel agar digit tidak dibulatkan. Nomor HP boleh kosong; saat import nomor kosong menghapus kontak sebelumnya. Nomor berawalan `08` atau `+62` diseragamkan menjadi `62`. Terapkan migration 000012 sebelum menjalankan versi ini.
