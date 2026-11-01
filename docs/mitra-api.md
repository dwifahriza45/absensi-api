# Mitra (admin)

Gunakan cookie login admin, dengan host yang sama saat login dan request. Base URL lokal: `http://127.0.0.1:8082/api/v1/admin/mitra`.

| Method | Path | Fungsi |
| --- | --- | --- |
| POST | `/api/v1/admin/mitra` | Create, HTTP 201 |
| GET | `/api/v1/admin/mitra` | List; filter opsional `?q=...` |
| GET | `/api/v1/admin/mitra/{mitra_id}` | Detail |
| PUT | `/api/v1/admin/mitra/{mitra_id}` | Update nama |
| DELETE | `/api/v1/admin/mitra/{mitra_id}` | Hapus mitra yang tidak digunakan |
| GET | `/api/v1/admin/mitra/export` | Download `mitra.xlsx`; filter `?q=...` |
| GET | `/api/v1/admin/mitra/import/template` | Download template kosong |
| POST | `/api/v1/admin/mitra/import` | Upload Excel atau JSON rows |

Body create/update:

```json
{"name":"Dinas Pendidikan"}
```

Nama wajib, maksimal 80 karakter. Nama yang sama persis menghasilkan 409. UUID `id` tetap ada, sedangkan `mitra_id` dibuat otomatis format `MTR-YYYYMMDD-000001`. Path dan relasi employee memakai kode INT, bukan UUID. Rename master langsung tercermin pada nama mitra employee/rekap.

Mitra yang masih direferensikan employee, termasuk employee nonaktif, tidak bisa dihapus (409). Pindahkan employee ke mitra lain dahulu. Foreign key database menjaga konsistensi dan write memakai transaksi/lock yang sama dengan import/edit employee. Tabel serta ID mitra yang sudah ada digunakan langsung, tanpa migration/data duplikat.

## Excel

Export dan template berisi satu kolom **NAMA MITRA** pada sheet **Mitra**. Header huruf besar, rata tengah horizontal/vertikal, background biru tua dan font putih tebal. User cukup mengisi nama mulai A2; kode tidak perlu diisi.

Di Postman: **POST** endpoint import → **Body → form-data** → key `file` bertipe **File**, pilih `.xlsx`. Biarkan Postman membuat Content-Type multipart/boundary otomatis.

Hasil import memakai envelope standar (`code`, `status`, `message`, `data`, `error`). `data` berisi `total`, jumlah `success`, `skip`, `error`, serta `rows` dengan nomor baris Excel, nama, kode hasil dan pesan status.

- `success`: nama baru berhasil dibuat, kode INT otomatis.
- `skip`: nama sudah terdaftar atau baris sama berulang dalam file.
- `error`: nama kosong, terlalu panjang, formula atau data baris yang tidak valid.

Export bisa di-import ulang; nama yang sama di-skip sehingga tidak menggandakan data. Mengubah nama pada file satu kolom dianggap menambah mitra baru; rename mitra lama dilakukan lewat CRUD update. Import tidak menghapus mitra yang tidak ada di file.

Batas 8 MiB/file, 32 MiB isi ZIP, 5.000 baris termasuk baris kosong. Format/header rusak ditolak sebelum perubahan DB. Formula, macro dan external-link workbook ditolak. Export menulis sel string agar nama tidak dianggap formula. Berkas tidak disimpan permanen. Maksimal 2 import bersamaan dengan antrean 5 detik, kemudian 503 untuk dicoba lagi.

Error per baris tidak menggagalkan baris valid. Kegagalan infrastruktur/timeout menggagalkan transaksi keseluruhan; hasil sukses baru dikirim setelah commit. HTTP 200 berarti proses selesai; FE tetap perlu membaca jumlah `data.error` (berbeda dari boolean `error` pada envelope).

JSON juga tersedia untuk FE yang membaca spreadsheet sendiri:

```json
{"rows":[{"name":"Dinas Pendidikan"},{"name":"Dinas Kesehatan"}]}
```

JSON boleh menyertakan `mitra_id` untuk update nama master; data identik di-skip, kode tidak ditemukan atau nama bentrok dilaporkan error per baris. Excel dua kolom `Kode Mitra` / `Nama Mitra` juga diterima, tetapi template/export untuk user hanya memakai satu kolom nama.

List employee mendukung `?mitra_id=MTR-YYYYMMDD-000001` selain filter nama `?mitra=...`.

Response list/detail/create/update juga memuat `employee_count`, `can_delete`, dan `delete_reason` saat penghapusan tidak diizinkan. Hitungan mencakup seluruh employee, termasuk nonaktif. Frontend menggunakan metadata ini untuk menonaktifkan ikon hapus dan menampilkan tooltip alasan. Foreign key tetap menolak delete bila relasi baru muncul setelah halaman dimuat.
