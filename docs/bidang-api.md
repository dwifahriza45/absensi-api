# CRUD bidang (admin)

Base URL: `http://127.0.0.1:8082/api/v1/admin/bidang`. Gunakan cookie login admin pada host yang sama.

| Method | Path | Hasil |
| --- | --- | --- |
| POST | `/api/v1/admin/bidang` | Create, HTTP 201 |
| GET | `/api/v1/admin/bidang` | List, filter opsional `?q=IT` |
| GET | `/api/v1/admin/bidang/{bidang_id}` | Detail |
| PUT | `/api/v1/admin/bidang/{bidang_id}` | Update nama |
| DELETE | `/api/v1/admin/bidang/{bidang_id}` | Hapus bidang yang tidak digunakan |

Body create/update:

```json
{"name":"Teknologi Informasi"}
```

Nama wajib diisi, maksimal 60 karakter. Nama yang sama persis menghasilkan 409. Create menghasilkan UUID `id` dan kode otomatis `bidang_id` format `BID-YYYYMMDD-000001`; semua path dan foreign key memakai kode BID. Response mengikuti format code/status/message/data/error dan valid untuk pesan validasi field.

Pada create/update employee, kirim kode hasil create bidang:

```json
{
  "name":"Budi Santoso",
  "phone":"081234567890",
  "mitra_id":"MTR-20261031-000001",
  "bidang_id":"BID-20261031-000001"
}
```

`bidang_id` wajib pada create/update employee. Kode yang tidak valid/tidak ditemukan menghasilkan 400. Field nama bebas `bidang` pada CRUD employee diganti dengan `bidang_id`. Response employee tetap memuat label `bidang` beserta kode `bidang_id`. Rename master langsung tercermin pada label profil/rekap.

Delete bidang yang masih direferensikan employee, termasuk employee nonaktif, menghasilkan 409. Pindahkan atau lepaskan bidang employee terlebih dahulu. Constraint FK di database mencegah relasi putus saat request bersamaan. Semua mutation memakai transaction/lock dengan batas tunggu, dan ID dialokasikan lewat counter atomik yang sama dengan mitra.

Migration 000008 menyalin label bidang lama ke master (trim spasi luar, gabungkan label yang sama), lalu mengisi `employees.bidang_id` dan menghapus kolom teks lama. Label kosong tetap NULL tanpa master. Down migration memulihkan nama bidang ke employee. Import Excel tetap menerima label `bidang`, mencari/membuat master di transaksi import.

## Import dan export Excel

| Method | Path | Hasil |
| --- | --- | --- |
| GET | `/api/v1/admin/bidang/export` | Download `bidang.xlsx`; mendukung filter `?q=...` |
| GET | `/api/v1/admin/bidang/import/template` | Download template kosong `bidang-template.xlsx` |
| POST | `/api/v1/admin/bidang/import` | Upload XLSX atau JSON rows; ringkasan dan status per baris |

Semua endpoint khusus admin. Di Postman, pilih Body → form-data, key `file` bertipe File, lalu pilih file `.xlsx`. Biarkan Postman membuat Content-Type multipart beserta boundary; jangan menuliskannya manual.

Gunakan sheet **Bidang**, dengan satu kolom **NAMA BIDANG** seperti template/export. Header berwarna biru tua dengan tulisan putih tebal, huruf besar, dan rata tengah horizontal/vertikal. User cukup mengisi nama; kode BID dibuat otomatis oleh database.

Nama baru dibuat, sedangkan nama yang sudah terdaftar di-skip. Mengganti nama pada file satu kolom akan membuat bidang baru; untuk mengganti nama master yang sudah ada, gunakan CRUD update di aplikasi. Format file dua kolom yang pernah di-download sebelumnya tetap diterima untuk kompatibilitas.

- `success`: bidang baru dibuat atau nama bidang berubah.
- `skip`: data sama, nama baru sudah terdaftar, atau baris persis sama berulang dalam file.
- `error`: nama kosong/terlalu panjang, format ID salah, ID tidak ditemukan, nama bentrok dengan bidang lain saat update, formula, atau kolom data tambahan.

Ringkasan berada di `data`; `error` di dalam `data` adalah **jumlah baris gagal**, sedangkan `error` pada envelope adalah **status kegagalan request**. Import selesai memakai HTTP 200 meskipun sebagian baris error, sehingga FE perlu membaca ringkasan ini.

```json
{
  "code": 200,
  "status": "OK",
  "message": "Bidang Import Completed",
  "data": {
    "total": 3,
    "success": 1,
    "skip": 1,
    "error": 1,
    "rows": [
      {"row": 2, "bidang_id": "BID-20261031-000001", "name": "IT", "status": "success", "message": "Bidang berhasil disimpan."},
      {"row": 3, "bidang_id": "BID-20261031-000001", "name": "IT", "status": "skip", "message": "Baris duplikat dalam file."},
      {"row": 4, "name": "", "status": "error", "message": "Nama Bidang Wajib Diisi"}
    ]
  },
  "error": false
}
```

Nomor `row` mengikuti baris Excel, mulai 2 setelah header. Baris kosong di tengah data dilaporkan sebagai error. Baris diproses sesuai urutan file; jika kode yang sama diubah berkali-kali, perubahan valid terakhir yang berlaku. Import tidak menghapus bidang yang tidak ada dalam file.

Batas file 8 MiB, total isi ZIP setelah dibuka 32 MiB, maksimal 5.000 baris data termasuk baris kosong. Format/header rusak atau batas terlampaui menghasilkan 400 sebelum perubahan DB. Formula, macro, dan external-link workbook ditolak; export memakai sel string agar nama yang diawali `=` tidak berubah menjadi formula. Berkas tidak disimpan permanen. Pemrosesan import dibatasi 2 request bersamaan; antrean maksimal 5 detik sebelum respons 503 untuk dicoba kembali.

Seluruh perubahan memakai satu transaksi dan lock yang sama dengan edit/import employee. Savepoint mengisolasi konflik per baris. Kegagalan infrastruktur atau timeout menggagalkan transaksi keseluruhan dan menghasilkan error HTTP, sehingga tidak mengklaim baris berhasil sebelum commit.

FE yang sudah membaca Excel sendiri juga bisa mengirim `Content-Type: application/json`:

```json
{"rows":[{"name":"IT"},{"bidang_id":"BID-20261031-000001","name":"Teknologi Informasi"}]}
```

Response list/detail/create/update juga memuat `employee_count`, `can_delete`, dan `delete_reason` saat penghapusan tidak diizinkan. Hitungan mencakup seluruh employee, termasuk nonaktif. Frontend menggunakan metadata ini untuk menonaktifkan ikon hapus dan menampilkan tooltip alasan. Foreign key tetap menolak delete bila relasi baru muncul setelah halaman dimuat.
