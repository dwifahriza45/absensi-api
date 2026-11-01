# Migration database

Jalankan file sesuai nomor versi. Setiap versi memiliki pasangan `.up.sql` untuk menerapkan perubahan dan `.down.sql` untuk membatalkannya.
| Versi | Perubahan | Isi rollback |
|---|---|---|
| `000001_init` | Tabel pegawai, metadata foto, absensi, laporan kerja | Hapus tabel dari anak ke induk |
| `000002_admin_sessions` | Akun admin, akun awal development, sesi admin/karyawan | Hapus sesi dan akun admin |
| `000003_instansi` | Master instansi, pindahkan nama lama, foreign key pegawai | Pulihkan nama instansi ke kolom teks |
| `000004_public_ids` | Counter, fungsi generator, backfill dan trigger ID bisnis | Hapus ID bisnis/generator; UUID tetap ada |
| `000005_employee_login_index` | Index nama pegawai aktif untuk login | Hapus index tambahan |
| `000006_business_key_references` | Karyawan EMP, FK ke kode bisnis, sesi SES dengan pemilik FK | Pulihkan referensi UUID dan kode USER karyawan |
| `000007_employee_phone_encryption` | Nomor kontak terenkripsi AES-GCM | Hapus ciphertext kontak, hash tetap ada |
| `000008_bidang` | Master bidang BID, backfill label dan FK employee | Pulihkan label bidang ke employee |
| `000009_remove_employee_daerah` | Hapus kolom daerah dan ubah index filter employee | Kembalikan kolom kosong; nilai lama tidak dipulihkan |

| `000010_instansi_to_mitra` | Rename master menjadi mitra, FK mitra_id dan kode MTR; pertahankan UUID/data/counter | Pulihkan nama instansi, FK instansi_id dan kode INT |

## Struktur setelah seluruh migration
| Tabel | Kegunaan | ID bisnis |
|---|---|---|
| `users` | Akun admin dengan email/password bcrypt | `user_id`: `USER-YYYYMMDD-000001` |
| `employees` | Karyawan dengan nama dan hash nomor HP | `employee_id`: `EMP-YYYYMMDD-000001` |
| `mitra` | Master mitra karyawan | `mitra_id`: `MTR-YYYYMMDD-000001` |
| `bidang` | Master bidang karyawan | `bidang_id`: `BID-YYYYMMDD-000001` |
| `photos` | Metadata file foto di storage privat | `photo_id`: `PHO-YYYYMMDD-000001` |
| `attendances` | Absensi per karyawan/tanggal | `attendance_id`: `ATT-YYYYMMDD-000001` |
| `work_reports` | Foto hasil kerja per absensi/sesi | `work_report_id`: `WRP-YYYYMMDD-000001` |
| `sessions` | Sesi login admin dan karyawan | `session_id`: `SES-YYYYMMDD-000001` |
| `public_id_counters` | Nomor terakhir per prefix/tanggal | Primary key `(prefix, id_date)` |

Kolom `id` tetap UUID dan menjadi primary key, tetapi foreign key setelah 000006 mengacu ke kolom kode bisnis yang UNIQUE. Contoh: `employees.mitra_id → mitra.mitra_id`, `employees.bidang_id → bidang.bidang_id`, `attendances.employee_id → employees.employee_id`, `work_reports.attendance_id → attendances.attendance_id`, serta seluruh referensi foto → `photos.photo_id`.

`sessions.user_id` mengacu ke `users.user_id` untuk admin; `sessions.employee_id` mengacu ke `employees.employee_id` untuk karyawan. CHECK constraint memastikan tepat satu pemilik sesuai role. Kolom polymorphic `subject` lama dihapus. JWT menggunakan kode pemilik sebagai subject dan kode SES sebagai jti.

000001–000005 mencerminkan tahapan skema sebelumnya; 000006 memetakan data lama, mengganti prefix karyawan USER menjadi EMP, dan mengatur counter EMP. Tanggal/sufiks kode karyawan lama dipertahankan. Stop app lama selama upgrade; access token UUID lama perlu refresh atau login ulang. Refresh token valid tetap dipertahankan. Rollback 000006 mengembalikan relasi UUID dan mengalokasikan kode USER baru untuk karyawan agar tidak berbenturan dengan admin; kode EMP/SES pada client harus diperbarui setelah rollback.

## Menjalankan

Dari root project `absensi-api`. Service `migrate` tersedia di Compose dev dan production dengan profile `tools`, seperti coffee-api. Perintah `run --rm migrate` mengaktifkan service itu secara eksplisit dan menunggu DB sehat; `up` biasa hanya menjalankan app/DB:

```sh
# Development: terapkan seluruh migration yang belum dijalankan.
make dev-migrate

# Production: terapkan seluruh migration yang belum dijalankan.
docker compose run --rm migrate
```

Migration baru ditambahkan dengan nomor berikutnya. Nomor dan nama file yang sudah ada dipertahankan agar riwayat database dan referensi tes tetap konsisten. Rollback harus dari versi terbaru ke versi sebelumnya; rollback `000002`/`000001` menghapus data tabel terkait.

Akun awal pada `000002` hanya untuk development; aplikasi production menolak password bawaannya. Detail sesi dan konfigurasi production ada di [catatan keamanan](../docs/security.md).

Migration 000010 mengganti kode INT menjadi MTR dengan tanggal dan nomor urut yang sama, termasuk referensi karyawan dan counter. Jalankan upgrade saat aplikasi lama dihentikan, lalu gunakan endpoint `/api/v1/admin/mitra` dan field `mitra_id`.
