# Kontrak request dan response

Format JSON mengikuti `coffee-api/internal/shared/utils/response.go`. HTTP status dan `code` selalu sama. `status` adalah `OK` atau `NOK`, dan `error` adalah boolean. Field opsional `data`, `total`, `valid` hanya dikirim bila tersedia. Endpoint download foto tetap mengembalikan byte gambar, bukan envelope JSON.

## Contoh login admin

`POST /api/v1/admin/auth/login` dengan Content-Type application/json:

```json
{"email":"","password":""}
```

HTTP **400**:

```json
{
  "code": 400,
  "status": "NOK",
  "message": "Validation Failed",
  "error": true,
  "valid": {
    "email": "Email Wajib Diisi",
    "password": "Password Wajib Diisi"
  }
}
```

Field tidak dikirim, body `null`, dan string berisi spasi saja menghasilkan pesan wajib yang sama. Email yang terisi tetapi salah format menghasilkan `valid.email = "Format Email Tidak Valid"`. Password dibatasi 72 byte sesuai bcrypt; nilai password tidak dinormalisasi atau dicetak pada pesan. Email dibersihkan dari spasi di tepi.

Login berhasil memakai HTTP **200**:

```json
{
  "code": 200,
  "status": "OK",
  "message": "Login Successfully",
  "data": {"expiresAt":"2026-10-31T22:00:00+07:00"},
  "error": false
}
```

Token tetap dikirim melalui cookie HttpOnly, tidak dikembalikan dalam JSON. FE memakai `credentials: 'include'`. Login karyawan menerima `{fullname,phone}`; error field memakai `valid.fullname` dan `valid.phone`.

Kredensial yang bentuknya valid tetapi tidak cocok tetap HTTP 401 dengan `status: "NOK"`, `error: true`, dan pesan generik, agar tidak mengungkap akun mana yang terdaftar. JSON rusak/field asing/tipe field salah diberi HTTP 400 dengan pesan umum; tidak semua error mempunyai `valid`.

## Pemakaian di frontend

FE dapat langsung menampilkan `response.valid.email`, `response.valid.password`, atau field lain di bawah input terkait. Validasi bisnis tetap di backend; response sukses tidak membawa field `valid`. Error tanpa `valid` ditampilkan melalui `message`. Jangan menganggap seluruh respons memakai HTTP 200.

Validasi berlaku sebelum service/database. DTO memakai tag `validate` dan `label`, seperti `validate:"required,notblank,email" label:"Email"`, melalui go-playground/validator yang sama dengan coffee-api. String identitas dapat dinormalisasi lewat metode `Normalize`; password tidak diubah. Import mempertahankan perilaku partial import: struktur/ukuran rows divalidasi, baris yang tidak valid tetap dilaporkan lewat data `skipped` oleh service.

Permintaan pulang awal mengembalikan `valid.reason` bila kosong. Alasan checkout bersifat kondisional terhadap status absensi dan dapat menghasilkan HTTP 422 dengan `valid.reason`. Multipart laporan kosong mengembalikan `valid.session_no` dan `valid.photo` bersama. Angka koordinat tidak valid memakai key field terkait di `valid`.

Menambahkan request baru cukup memberi tag validasi di DTO; `httpx.DecodeJSON`/`DecodeJSONLimit` otomatis menjalankan validasi setelah parsing JSON yang ketat. Validasi field multipart dilakukan handler setelah parsing form. Success/error handler, middleware autentikasi, dan error domain memakai helper response yang sama.

Referensi validator: [go-playground/validator v10.30.4](https://pkg.go.dev/github.com/go-playground/validator/v10@v10.30.4). Dependency mimetype dipin ke versi unretracted v1.4.16.

## Batas tunggu logout

Operasi pencabutan sesi pada logout dibatasi 10 detik, termasuk menunggu koneksi pool dan query DELETE. Jika batas tercapai, endpoint mengembalikan HTTP 504 dengan `code: 504`, `status: "NOK"`, `error: true`, pesan `Batas waktu logout tercapai. Silakan coba lagi.`, dan header `Retry-After: 2`.

Cookie hanya dibersihkan setelah pencabutan sesi berhasil dikonfirmasi. Timeout tidak boleh dianggap sebagai logout sukses; bila hasil commit belum pasti, client bisa mengulangi logout yang bersifat idempotent. Batas backend ini berlaku setelah request mencapai handler. Di Postman, gunakan Request timeout 15000 ms agar koneksi yang belum mencapai backend juga dibatasi oleh client.
